package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (app *application) routes() http.Handler {
	chain := app.DynChain()

	router := httprouter.New()

	router.Handler(http.MethodGet, "/assets/", app.FileServer())
	router.Handler(http.MethodGet, "/", chain.ThenFunc(app.home))

	return app.StdChain().Then(router)
}
