package bow

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

type contextKey int

const (
	contextKeyLayout contextKey = iota

	partialPrefix = "_"
	layoutsFolder = "layouts"
)

// ReqFuncMap is a dynamic version of template.FuncMap that is request-aware.
type ReqFuncMap map[string]func(r *http.Request) interface{}

// Views is an engine that will render Views from templates.
type Views struct {
	Logger *log.Logger
	Debug  bool // to display errors in http responses

	pages    map[string]*template.Template
	partials map[string]*template.Template

	funcs    template.FuncMap
	reqFuncs ReqFuncMap
}

// NewViews creates a views engine.
func NewViews() *Views {
	views := Views{
		Logger: log.New(os.Stdout, "", log.Ldate|log.Ltime),

		pages:    make(map[string]*template.Template),
		partials: make(map[string]*template.Template),
		funcs:    make(template.FuncMap),
		reqFuncs: make(ReqFuncMap),
	}

	views.Funcs(template.FuncMap{
		"safe": safe,
	})

	views.ReqFuncs(ReqFuncMap{
		"partial": views.partial,
	})

	return &views
}

// safe returns a verbatim unescaped HTML from a string.
func safe(s string) template.HTML {
	return template.HTML(s)
}

// partial is meant to be added as a ReqFuncMap to include partials from within templates.
func (views *Views) partial(r *http.Request) interface{} {
	return func(name string, data interface{}) (template.HTML, error) {
		partial, ok := views.partials[name]
		if !ok {
			return "", fmt.Errorf("partial %s not found", name)
		}

		var buf bytes.Buffer
		if err := views.renderTemplate(&buf, r, partial, "main", data); err != nil {
			return "", err
		}
		return template.HTML(buf.String()), nil
	}
}

// Funcs adds the elements of the argument map to the list of
// functions to inject into templates.
func (views *Views) Funcs(funcs template.FuncMap) {
	for k, fn := range funcs {
		views.funcs[k] = fn
	}
}

// ReqFuncs adds the elements of the argument map to the list of
// request-aware functions to inject into templates.
func (views *Views) ReqFuncs(reqFuncs ReqFuncMap) {
	for k, fn := range reqFuncs {
		views.reqFuncs[k] = fn

		// also add a corresponding empty function to funcs
		// that will be redifined at the rendering with the request info.
		views.funcs[k] = func() string { return "" }
	}
}

// Parse walks a filesystem from the root folder to discover and parse
// html files into views. Files starting with an underscore are partial views.
// Files in the layouts folder not starting with underscore are layouts. The rest of
// html files are full page views. The funcs parameter is a list of functions that is
// attached to views.
//
// Views, layouts and partials will be referred to with their path, but without the
// root folder, and without the file extension.
//
// Layouts will be referred to without the layouts folder neither.
//
// Partials files are named with a leading underscore to distinguish them from regular views,
// but will be referred to without the underscore.
func (views *Views) Parse(fsys fs.FS) error {
	var pages, partials, layouts []string

	err := fs.WalkDir(fsys, "views", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}

		dirs := strings.Split(filepath.Dir(path), string(os.PathSeparator))

		switch {
		case filepath.Base(path)[0:1] == partialPrefix:
			partials = append(partials, path)
		case len(dirs) > 1 && dirs[1] == layoutsFolder:
			layouts = append(layouts, path)
		default:
			pages = append(pages, path)
		}

		return nil
	})
	if err != nil {
		return err
	}

	for _, page := range pages {
		tmpl, err := parseTemplate(fsys, views.funcs, page, layouts)
		if err != nil {
			return err
		}

		views.pages[templateName(page)] = tmpl
	}

	for _, partial := range partials {
		tmpl, err := parseTemplate(fsys, views.funcs, partial, nil)
		if err != nil {
			return err
		}

		views.partials[templateName(partial)] = tmpl
	}

	return nil
}

// parseTemplate creates a new template from the given path and parses the main and
// associated templates from the given filesystem. It also attached funcs.
func parseTemplate(fsys fs.FS, funcs template.FuncMap, main string, associated []string) (*template.Template, error) {
	tmpl := template.New("main").Funcs(funcs)

	if main != "" {
		b, err := fs.ReadFile(fsys, main)
		if err != nil {
			return nil, err
		}

		_, err = tmpl.Parse(string(b))
		if err != nil {
			return nil, err
		}
	}

	for _, path := range associated {
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, err
		}

		_, err = tmpl.New(templateName(path)).Parse(string(b))
		if err != nil {
			return nil, err
		}
	}

	return tmpl, nil
}

// templateName returns a template name from a path.
// It removes the extension, removes the leading "_" from partials
// and trims the root directory.
func templateName(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))

	if base[0:1] == partialPrefix {
		base = base[1:]
	}

	dirs := strings.Split(filepath.Dir(path), string(os.PathSeparator))
	dir := filepath.Join(dirs[1:]...)

	return filepath.Join(dir, base)
}

// Render renders a given view or partial.
//
// For page views, the layout can be set using the WithLayout function or using the ApplyLayout middleware.
// If no layout is defined, the "base" layout will be chosen. Partial views are rendered without any layout.
func (views *Views) Render(w http.ResponseWriter, r *http.Request, status int, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html")

	partial, ok := views.partials[name]
	if ok {
		if err := views.renderTemplate(w, r, partial, "main", data); err != nil {
			views.ServerError(w, err)
		}
		return
	}

	view, ok := views.pages[name]
	if !ok {
		views.ServerError(w, fmt.Errorf("view %s not found", name))
		return
	}

	layout, ok := r.Context().Value(contextKeyLayout).(string)
	if ok {
		layout = filepath.Join(layoutsFolder, layout)
	} else {
		layout = filepath.Join(layoutsFolder, "base")
	}

	if view.Lookup(layout) == nil {
		views.ServerError(w, fmt.Errorf("layout %s not found", layout))
		return
	}

	// write http status code
	w.WriteHeader(status)

	if err := views.renderTemplate(w, r, view, layout, data); err != nil {
		views.ServerError(w, err)
	}
}

// renderTemplate injects dynamic funcs and renders the given template using a buffer to catch runtime errors.
func (views *Views) renderTemplate(w io.Writer, r *http.Request, tmpl *template.Template, name string, data interface{}) error {
	tmpl, err := tmpl.Clone()
	if err != nil {
		return err
	}

	for k, fn := range views.reqFuncs {
		views.funcs[k] = fn(r)
	}

	tmpl.Funcs(views.funcs)

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return err
	}

	if _, err := buf.WriteTo(w); err != nil {
		return err
	}

	return nil
}

// ServerError writes an error message and stack trace to the logger,
// then sends a generic 500 Internal Server Error response to the user.
func (views *Views) ServerError(w http.ResponseWriter, err error) {
	trace := fmt.Sprintf("%s\n%s", err.Error(), debug.Stack())
	views.Logger.Output(2, trace)

	if views.Debug {
		http.Error(w, trace, http.StatusInternalServerError)
		return
	}

	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// ClientError sends a specific status code and corresponding description to the user.
// This should be used to send responses when there's a problem with the request that the user sent.
func (views *Views) ClientError(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

// WithLayout returns a shallow copy of the request but with the information of the layout to apply.
// It can be used in a handler before calling render to change the layout.
func WithLayout(r *http.Request, layout string) *http.Request {
	ctx := context.WithValue(r.Context(), contextKeyLayout, layout)
	return r.WithContext(ctx)
}

// ApplyLayout is a middleware that applies a specific layout for the rendering of the view.
// It returns a function which has the correct signature to be used with alice, but it can
// also be used without.
//
// https://pkg.go.dev/github.com/justinas/alice#Constructor
func ApplyLayout(layout string) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.ServeHTTP(w, WithLayout(r, layout))
		})
	}
}
