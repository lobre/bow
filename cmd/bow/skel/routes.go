package main

import (
	"net/http"

	"github.com/bmizerany/pat"
)

func (app *application) routes() http.Handler {
	chain := app.DynChain()

	mux := pat.New()

	mux.Get("/assets/", app.FileServer())
	mux.Get("/", chain.ThenFunc(app.home))

	return app.StdChain().Then(mux)
}
