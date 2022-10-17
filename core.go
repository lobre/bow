package bow

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/benbjohnson/hashfs"
	"github.com/golangcollege/sessions"
	"github.com/justinas/alice"
	"github.com/justinas/nosurf"
)

// Core holds the core logic to configure and run a simple web app.
// It is meant to be embedded in a parent web app structure.
type Core struct {
	fsys  fs.FS
	hfsys *hashfs.FS

	Logger *log.Logger

	DB      *DB
	Views   *Views
	Session *sessions.Session

	translator *Translator
	csp        map[string]string
}

// NewCore creates a core with sane defaults. Options can be used for specific configurations.
func NewCore(fsys fs.FS, options ...Option) (*Core, error) {
	hfsys := hashfs.NewFS(fsys)

	core := &Core{
		Logger: log.New(os.Stdout, "", log.Ldate|log.Ltime),

		csp: map[string]string{
			"default-src": "'self'",
		},

		fsys:  fsys,
		hfsys: hfsys,

		Views: NewViews(),
	}

	for _, opt := range options {
		if err := opt(core); err != nil {
			return nil, err
		}
	}

	// reapply logger to match the one provided as option
	core.Views.Logger = core.Logger

	// set default funcs
	core.Views.Funcs(template.FuncMap{
		"hash": hfsys.HashName,
		"format": func(layout string, dt time.Time) string {
			return dt.Format(layout)
		},
	})

	// set default req funcs
	core.Views.ReqFuncs(ReqFuncMap{
		"csrf": func(r *http.Request) interface{} {
			return func() string {
				return nosurf.Token(r)
			}
		},
	})

	if err := core.Views.Parse(fsys); err != nil {
		return nil, err
	}

	return core, nil
}

// Option configures a core.
type Option func(*Core) error

// WithLogger is an option to set the application logger.
func WithLogger(logger *log.Logger) Option {
	return func(core *Core) error {
		core.Logger = logger
		return nil
	}
}

// WithCSP is an option to set the csp rules that will be set on http responses.
// By default, only default-src : 'self' is defined.
func WithCSP(csp map[string]string) Option {
	return func(core *Core) error {
		for k, v := range csp {
			core.csp[k] = v
		}
		return nil
	}
}

// WithDebug is an option to spit the server errors directly in
// http responses, instead of a generic 'Internal Server Error' message.
func WithDebug(debug bool) Option {
	return func(core *Core) error {
		core.Views.Debug = debug
		return nil
	}
}

// WithDB is an option to enable and configure the database access.
func WithDB(dsn string) Option {
	return func(core *Core) error {
		core.DB = NewDB(dsn, core.fsys)
		if err := core.DB.Open(); err != nil {
			return err
		}
		return nil
	}
}

// WithTranslator is an option to enable and configure the translator.
// If the locale paramater value is "auto", the locale will be retrieved
// first from the "lang" cookie, then from the "Accept-Language" request header.
// If it cannot retrieve it, messages will be returned untranslated.
func WithTranslator(locale string) Option {
	return func(core *Core) error {
		core.translator = NewTranslator()
		if err := core.translator.Parse(core.fsys); err != nil {
			return err
		}

		if locale != "auto" {
			core.Views.Funcs(template.FuncMap{
				"translate": func(msg string) string {
					return core.translator.Translate(msg, locale)
				},
				"lang": func() string {
					return core.translator.langFromLocale(locale)
				},
				"format": func(layout string, dt time.Time) string {
					return Format(dt, layout, locale)
				},
			})
		} else {
			core.Views.ReqFuncs(ReqFuncMap{
				"translate": func(r *http.Request) interface{} {
					return func(msg string) string {
						return core.translator.Translate(msg, core.translator.ReqLocale(r))
					}
				},
				"lang": func(r *http.Request) interface{} {
					return func() string {
						return core.translator.langFromLocale(core.translator.ReqLocale(r))
					}
				},
				"format": func(r *http.Request) interface{} {
					return func(layout string, dt time.Time) string {
						return Format(dt, layout, core.translator.ReqLocale(r))
					}
				},
			})
		}

		return nil
	}
}

// WithSession is an option to enable cookie sessions.
// The key parameter is the secret you want to use to authenticate
// and encrypt sessions cookies, and should be 32 bytes long.
func WithSession(key string) Option {
	return func(core *Core) error {
		core.Session = sessions.New([]byte(key))
		core.Session.Lifetime = 12 * time.Hour

		core.Views.ReqFuncs(ReqFuncMap{
			"flash": func(r *http.Request) interface{} {
				return func() string {
					return core.Session.PopString(r, "flash")
				}
			},
		})

		return nil
	}
}

// WithFuncs is an option to configure default functions that will
// be injected into views.
func WithFuncs(funcs template.FuncMap) Option {
	return func(core *Core) error {
		for k, fn := range funcs {
			core.Views.funcs[k] = fn
		}
		return nil
	}
}

// WithReqFuncs is an option similar to WithFuncs, but with functions that
// are request-aware.
func WithReqFuncs(funcs ReqFuncMap) Option {
	return func(core *Core) error {
		for k, fn := range funcs {
			core.Views.reqFuncs[k] = fn
		}
		return nil
	}
}

// WithGlobals is an option that allows to define a function that is
// called at each rendering to inject data that can be retrieved using the
// "globals" helper template function.
func WithGlobals(fn func(*http.Request) interface{}) Option {
	return func(core *Core) error {
		core.Views.ReqFuncs(ReqFuncMap{
			"globals": func(r *http.Request) interface{} {
				return func() interface{} {
					return fn(r)
				}
			},
		})
		return nil
	}
}

// FileServer returns a handler for serving filesystem files.
// It enforces http cache by appending hashes to filenames.
// A hashName function is defined in templates to gather the hashed filename of a file.
func (core *Core) FileServer() http.Handler {
	return hashfs.FileServer(core.hfsys)
}

// StdChain returns a chain of middleware that can be applied to all routes.
// It gracefully handles panics to avoid spinning down the whole app.
// It logs requests and add default secure headers.
func (core *Core) StdChain() alice.Chain {
	return alice.New(
		core.recoverPanic,
		core.logRequest,
		core.secureHeaders,
		methodOverride,
	)
}

// DynChain returns a chain of middleware that can be applied to all dynamic routes.
// It injects a CSRF cookie and enable sessions.
func (core *Core) DynChain() alice.Chain {
	chain := alice.New(injectCSRFCookie)
	if core.Session != nil {
		chain = chain.Append(core.Session.Enable)
	}
	return chain
}

// logRequest is a middleware that logs the request to the application logger.
func (core *Core) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		core.Logger.Printf("%s - %s %s %s", r.RemoteAddr, r.Proto, r.Method, r.URL.RequestURI())
		next.ServeHTTP(w, r)
	})
}

// recoverPanic is a middleware that gracefully handles any panic that happens in the
// current go routine.
// By default, panics don't shut the entire application (only the current go routine),
// but if one arise, the server will return an empty response. This middleware is taking
// care of recovering the panic and sending a regular 500 server error.
func (core *Core) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// make the http.Server automatically close the current connection.
				w.Header().Set("Connection", "close")
				core.Views.ServerError(w, fmt.Errorf("%s", err))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// secureHeaders is a middleware that injects headers in the response
// to prevent XSS and Clickjacking attacks.
func (core *Core) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		for k, v := range core.csp {
			fmt.Fprintf(&b, "%s %s;", k, v)
		}

		w.Header().Set("Content-Security-Policy", b.String())
		w.Header().Set("Referrer-Policy", "origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")
		w.Header().Set("X-XSS-Protection", "0")

		next.ServeHTTP(w, r)
	})
}

// methodOverride is a middleware to allow to spoof the HTTP method.
// As html form only allow GET and POST, it allows the developer to extend
// that to PUT, PATCH and DELETE using a hidden input in the form.
//
// <input type="hidden" name="_method" value="PUT">
func methodOverride(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// only act on POST requests
		if r.Method == "POST" {
			method := r.PostFormValue("_method")
			if method == "" {
				method = r.Header.Get("X-HTTP-Method-Override")
			}

			// make sure it is a valid http method
			if method == "PUT" || method == "PATCH" || method == "DELETE" {
				r.Method = method
			}
		}

		next.ServeHTTP(w, r)
	})
}

// injectCSRFCookie is a middleware that injects an encrypted CSRF token in a cookie.
// That same token is used as a hidden field in forms (from nosurf.Token()).
// On the form submission, the server checks that these two values match.
// So directly trying to post a request to our secured endpoint without this parameter would fail.
// The only way to submit the form is from our frontend.
func injectCSRFCookie(next http.Handler) http.Handler {
	csrfHandler := nosurf.New(next)
	csrfHandler.SetBaseCookie(http.Cookie{
		HttpOnly: true,
		Path:     "/",
	})

	return csrfHandler
}

// Flash sets a flash message to the session.
func (core *Core) Flash(r *http.Request, msg string) {
	core.Session.Put(r, "flash", msg)
}

// Run runs the http server and launches a goroutine
// to listen to os.Interrupt before stopping it gracefully.
func (core *Core) Run(srv *http.Server) error {
	shutdown := make(chan error)

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt)
		<-stop

		core.Logger.Println("shutting down server")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		shutdown <- srv.Shutdown(ctx)
	}()

	core.Logger.Printf("starting server on %s\n", srv.Addr)

	err := srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	err = <-shutdown
	if err != nil {
		return err
	}

	core.Logger.Println("server stopped")

	return nil
}
