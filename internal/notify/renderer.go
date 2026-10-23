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

type Renderer struct {
	template *template.Template
}

func NewRenderer(text string) (*Renderer, error) {
	if text == "" {
		text = defaultTemplate
	}
	parsed, err := template.New("notification").Option("missingkey=error").Parse(text)
	if err != nil {
		return nil, fmt.Errorf("invalid notification template: %w", err)
	}
	return &Renderer{template: parsed}, nil
}

func (r *Renderer) Render(view ReportView) (string, error) {
	var body strings.Builder
	if err := r.template.Execute(&body, view); err != nil {
		return "", fmt.Errorf("render notification template: %w", err)
	}
	return body.String(), nil
}
