package main

import (
	"net/http"

	"github.com/lobre/bow"
)

type templateData struct {
	Form *bow.Form

	// add your template data
}

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	app.Views.Render(w, r, "home", templateData{})
}
