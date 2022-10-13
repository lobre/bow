package main

import (
	"net/http"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	app.Views.Render(w, r, "home", templateData{})
}
