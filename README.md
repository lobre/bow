# bow

Bow wraps a minimal amount of go libraries to quickly bootstrap a capable web project.

## What’s more than net/http?

## Getting started

### Using bow cli

That is the quickest way to get you started. First, install the cli.

```
go install github.com/lobre/bow/cmd/bow@latest
```

Initialize a new project.

```
mkdir myproject && cd myproject
go mod init myproject
```

Generate all the necessary files.

```
bow init
```

Tidy your dependencies and build the project.

```
go mod tidy
go build
```

Run and check your browser at [localhost:8080](https://localhost:8080).

```
./myproject
```

You are now ready to start developing features!

### Manually

<details>
  <summary>See details</summary>

  First, initialize a new project.
  
  ```
  mkdir myproject && cd myproject
  go mod init myproject
  ```
  
  Gather the dependencies.
  
  ```
  go get github.com/bmizerany/pat
  go get github.com/lobre/bow
  ```
  
  You will then need to define a base HTML layout.
  
  ```
  mkdir -p views/layouts
  cat views/layouts/base.html
  
  <!DOCTYPE html>
  <html lang="us">
    <head>
      <meta charset="utf-8" />
      <meta name="csrf-token" content="{{ csrf }}" />
      <meta name="viewport" content="width=device-width, initial-scale=1, minimum-scale=1" />
  
      <title>{{ template "title" . }}</title>
  
      <link href='/{{ hash "assets/style.css" }}' rel="stylesheet">
      <link rel="icon" href='/{{ hash "assets/favicon.ico" }}'>
  
      {{ block "head"  . }}{{ end }}
    </head>
  
    <body>
      <nav>
        <a href="/">{{ "Home" }}</a>
      </nav>
  
      <main>
        {{ template "main" . }}
      </main>
    </body>
  </html>
  ```
  
  > **_NOTE:_** The folders `views` and `layouts` cannot be renamed, and the base layout should be named `base.html`.
  
  Then, let’s create a first HTML page.
  
  ```
  cat views/home.html
  
  {{ define "title" }}{{ "Home" }}{{ end }}
  
  <div>"Hello World"</div>
  ```
  
  Now, let’s create an assets folder in which you can add your favicon and your css style which will be empty for now.
  
  ```
  mkdir assets
  cp <your_icon> assets/favicon.ico
  touch assets/style.css
  ```
  
  It is now the time to start implementing our go code! Create a `main.go`.
  
  You will need a `fs.FS` for those just created assets and templates. It is recommended to use an embed, so that they will be contained in your final binary. Add this at the top of your `main.go` file.
  
  ```
  //go:embed assets
  //go:embed views
  var fsys embed.FS
  ```
  
  Bow brings a `bow.Core` structure that should be embedded in your own struct. I have defined this struct `application` here in the `main.go` as well.
  
  ```
  type application struct {
  	*bow.Core
  
  	// your future own fields
  }
  ```
  
  Then create your main func, define an instance of this struct and configure bow.
  
  ```
  func main() {
  	app := application{}
  	app.Core, err_ = bow.NewCore(fsys)
  	if err != nil {
  		panic(err)
  	}
  }
  ```
  
  We now need to define our application routes. Add this other function to your `main.go`.
  
  ```
  func (app *application) routes() http.Handler {
  	chain := app.DynChain()
  	mux := pat.New()
  	mux.Get("/assets/", app.FileServer())
  	mux.Get("/", chain.ThenFunc(app.home))
  	return app.StdChain().Then(mux)
  }
  ```
  
  And also our home handler that tells to render the page named `home` and that will correspond to our `views/home.html`.
  
  ```
  func (app *application) home(w http.ResponseWriter, r *http.Request) {
  	app.Views.Render(w, r, "home", nil)
  }
  ```
  
  To finish, at the end of your `main.go`, create an `http.Server` and run the app.
  
  ```
  func main() {
  	app := application{}
  	app.Core, err_ = bow.NewCore(fsys)
  	if err != nil {
  		panic(err)
  	}
  	
  	srv := &http.Server{
  		Addr:         ":8080",
  		Handler:      app.routes(),
  		IdleTimeout:  time.Minute,
  		ReadTimeout:  10 * time.Second,
  		WriteTimeout: 30 * time.Second,
  	}
  	
  	err := app.Run(srv)
  	if err != nil {
  		panic(err)
  	}
  }
  ```
  
  > **_NOTE:_** Make sure to format your `main.go` and auto-import the dependencies.
  
  Build the project.
  
  ```
  go build
  ```
  
  Finally, run and check your browser at [localhost:8080](https://localhost:8080).
  
  ```
  ./myproject
  ```
  
  You are now ready to start developing features!
</details>

## Features

## Framework or not?

> When you use a library, you are in charge of the application flow. You choose when and where to call the library. When you use a framework, the framework is in charge of the flow.

Following that definition, I don’t consider bow to be a web framework. It is simply a set of libraries that are carefully wrapped to provide the necessary tools required to build a robust web application. You simply embed the main `bow.Core` structure in your application, and you still have the freedom to organize your go code as you wish.

## Dependencies

The goal of the project was to a minimal set of dependencies.

The choice of those dependencies has been made carefully to include only small, strongly built and focused libraries that were not worth reimplementing.

- [benbjohnson/hashfs](https://github.com/benbjohnson/hashfs): Append hashes to filename for better HTTP caching.
- [github.com/bmizerany/pat](https://github.com/bmizerany/pat): A simple pattern muxer for net/http.
- [golangcollege/sessions](https://github.com/golangcollege/sessions): Ligthweight HTTP session cookie implementation.
- [goodsign/monday](https://github.com/goodsign/monday): Minimalist translator for month and day of week names.
- [justinas/alice](https://github.com/justinas/alice): Easily chain your HTTP middleware functions.
- [justinas/nosurf](https://github.com/justinas/nosurf): Middleware to prevent Cross-Site Request Foregy attacks.
- [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3): A robust sqlite3 driver.

## Acknowledgement

This project has been heavily inspired by the awesome work of Alex Edwards documented in his book [Let’s Go](https://lets-go.alexedwards.net/).

## Projects

For a project using bow, check [github.com/lobre/tdispo](https://github.com/lobre/tdispo).
