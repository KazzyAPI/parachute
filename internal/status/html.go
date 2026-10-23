package status

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

const page = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Parachute status</title>
<style>
body{font-family:system-ui,sans-serif;margin:1rem;color:#111;background:#fff}
table{border-collapse:collapse;margin-bottom:1.5rem;font-size:.85rem}
th,td{border:1px solid #bbb;padding:.25rem .5rem;text-align:left;vertical-align:top;word-break:break-all}
th{background:#eee}
</style>
</head>
<body>
<h1>Parachute status</h1>
<p>armed: {{text .armed}} &middot; generated: {{text .generated}} &middot; last cycle: {{text .last_cycle}}</p>
{{range .hosts}}<section>
<h2>Host {{text .name}} (reachable: {{text .reachable}})</h2>
{{range .stacks}}<h3>Stack {{text .name}}</h3>
<ul>{{range $key, $value := .}}{{if and (ne $key "services") (ne $key "name")}}<li>{{$key}}: {{text $value}}</li>{{end}}{{end}}</ul>
<table>
{{with .services}}<tr>{{range $key, $value := index . 0}}<th>{{$key}}</th>{{end}}</tr>{{end}}
{{range .services}}<tr>{{range $key, $value := .}}<td>{{text $value}}</td>{{end}}</tr>
{{end}}</table>
{{end}}</section>
{{end}}</body>
</html>
`

type HTML struct {
	page *template.Template
}

func NewHTML() *HTML {
	return &HTML{page: template.Must(template.New("status").Funcs(template.FuncMap{"text": fieldText}).Parse(page))}
}

func (h *HTML) Write(out io.Writer, document Document) error {
	encoded, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode status page: %w", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return fmt.Errorf("encode status page: %w", err)
	}
	if err := h.page.Execute(out, fields); err != nil {
		return fmt.Errorf("render status page: %w", err)
	}
	return nil
}

func fieldText(value any) string {
	switch typed := value.(type) {
	case nil:
		return "-"
	case string:
		return typed
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}
