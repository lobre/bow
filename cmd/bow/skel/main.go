package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/lobre/bow"
)

//go:embed assets
//go:embed views
{%- if .WithDB %}
//go:embed migrations/*.sql
{%- end %}
{%- if .WithTranslator %}
//go:embed translations/*.csv
{%- end %}
var fsys embed.FS

type config struct {
	port int
	debug bool
	{%- if .WithDB %}
	dsn string
	{%- end %}
	{%- if .WithSession %}
	sessionKey string
	{%- end %}
	{%- if .WithTranslator %}
	locale string
	{%- end %}
}

type application struct {
	*bow.Core

	config config
	{%- if .WithDB %}

	// TODO: define your repositories
	// userRepo *StatusRepo
	// ...
	{%- end %}
}

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	var cfg config

	fs := flag.NewFlagSet(args[0], flag.ExitOnError)

	fs.IntVar(&cfg.port, "port", 8080, "http server port")
	fs.BoolVar(&cfg.debug, "debug", false, "display errors in http responses")
	{%- if .WithDB %}
	fs.StringVar(&cfg.dsn, "dsn", "{% .Binary %}.db", "database data source name")
	{%- end %}
	{%- if .WithSession %}
	fs.StringVar(&cfg.sessionKey, "session-key", "xxx", "session key for cookies encryption")
	{%- end %}
	{%- if .WithTranslator %}
	fs.StringVar(&cfg.locale, "locale", "auto", "locale of the application")
	{%- end %}

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	app := application{
		config: cfg,
	}

	var err error

	app.Core, err = bow.NewCore(
		fsys,
		bow.WithDebug(cfg.debug),
		{%- if .WithDB %}
		bow.WithDB(cfg.dsn),
		{%- end %}
		{%- if .WithSession %}
		bow.WithSession(cfg.sessionKey),
		{%- end %}
		{%- if .WithTranslator %}
		bow.WithTranslator(cfg.locale),
		{%- end %}
	)
	if err != nil {
		return err
	}
	{%- if .WithDB %}

	// inject db into repositories
	// app.userRepo = &UserRepo{db: app.DB}
	{%- end %}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.port),
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	if err := app.Run(srv); err != nil {
		return err
	}

	{% if .WithDB -%}
	return app.DB.Close()
	{%- else -%}
	return nil
	{%- end %}
}
