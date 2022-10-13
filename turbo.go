package bow

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strings"
)

type StreamAction string

const (
	ActionAppend  StreamAction = "append"
	ActionPrepend              = "prepend"
	ActionReplace              = "replace"
	ActionUpdate               = "update"
	ActionRemove               = "remove"
	ActionBefore               = "before"
	ActionAfter                = "after"

	streamMime string = "text/vnd.turbo-stream.html"
)

// AcceptsStream returns true if the request has got a Accept header saying
// that it accepts turbo streams in response.
func AcceptsStream(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, streamMime)
}

// RenderStream renders a partial view and wraps it in a turbo stream tag.
// It also sets the appropriate Content-Type header on the response.
func (views *Views) RenderStream(action StreamAction, target string, w http.ResponseWriter, r *http.Request, name string, data interface{}) {
	w.Header().Set("Content-Type", streamMime)

	var buf bytes.Buffer

	if action != ActionRemove {
		partial, ok := views.partials[name]
		if !ok {
			views.ServerError(w, fmt.Errorf("partial %s not found", name))
			return
		}

		if err := views.renderWithFuncs(&buf, r, partial, name, data); err != nil {
			views.ServerError(w, err)
			return
		}
	}

	wrapper := `<turbo-stream action="{{ .Action }}" target="{{ .Target }}">
  <template>
    {{ .Content }}
  </template>
</turbo-stream>`

	tmpl := template.Must(template.New("stream").Parse(wrapper))

	stream := struct {
		Action  StreamAction
		Target  string
		Content template.HTML
	}{action, target, template.HTML(buf.String())}

	if err := renderBuffered(w, tmpl, "stream", stream); err != nil {
		views.ServerError(w, err)
	}
}
