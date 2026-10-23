package notify

import (
	"fmt"
	"strings"
	"text/template"
)

const defaultTemplate = `{{- range .Hosts}}
{{- if not .Reachable}}{{.Name}}: unreachable
{{else if .Stacks}}{{.Name}}
{{range .Stacks}}  {{.Name}}
{{range .Services}}    {{.Name}} ({{.Container}}): {{.State}}{{if .Reason}} ({{.Reason}}){{end}}{{if .HoldUntil}} until {{.HoldUntil}}{{end}} {{.Image}}{{if .TransactionID}} {{.TransactionID}}{{end}}
{{end}}{{end}}{{end}}{{end}}`

const defaultEntriesTemplate = `{{range .}}{{.Level}} {{.Message}}{{range $key, $value := .Fields}} {{$key}}={{$value}}{{end}}
{{end}}`

type Renderer struct {
	template *template.Template
}

func NewRenderer(text string, report bool) (*Renderer, error) {
	switch {
	case text != "":
	case report:
		text = defaultTemplate
	default:
		text = defaultEntriesTemplate
	}
	parsed, err := template.New("notification").Option("missingkey=error").Parse(text)
	if err != nil {
		return nil, fmt.Errorf("invalid notification template: %w", err)
	}
	return &Renderer{template: parsed}, nil
}

func (r *Renderer) Render(data any) (string, error) {
	var body strings.Builder
	if err := r.template.Execute(&body, data); err != nil {
		return "", fmt.Errorf("render notification template: %w", err)
	}
	return body.String(), nil
}
