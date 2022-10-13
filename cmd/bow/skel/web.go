package main

import (
	"net/http"

	"github.com/lobre/bow"
)

type templateData struct {
	Form *bow.Form

	// add your template data
}

// addGlobals automatically injects data that are common to all pages.
func (app *application) addGlobals(r *http.Request) interface{} {
	return struct{}{}
}
