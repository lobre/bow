package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (app *application) routes() http.Handler {
	// middleware chain applied to all dynamic routes
	dynamic := app.DynChain()

	router := httprouter.New()
	router.Handler(http.MethodGet, "/assets/*filepath", app.FileServer())

	// application routes
	router.Handler(http.MethodGet, "/", dynamic.ThenFunc(app.home))

	return app.StdChain().Then(router)
}
