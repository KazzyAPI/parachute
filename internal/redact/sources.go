package redact

import (
	"io"
	"net/url"
	"strconv"
	"strings"
)

func URLSecrets(addresses []string) Source {
	var secrets []string
	for _, address := range addresses {
		secrets = append(secrets, address)
		_, rest, found := strings.Cut(address, "://")
		if !found {
			continue
		}
		location, rawQuery, _ := strings.Cut(rest, "?")
		location, _, _ = strings.Cut(location, "#")
		rawQuery, _, _ = strings.Cut(rawQuery, "#")
		authority, path, _ := strings.Cut(location, "/")
		if at := strings.LastIndex(authority, "@"); at >= 0 {
			userinfo := authority[:at]
			_, password, _ := strings.Cut(userinfo, ":")
			secrets = append(secrets, userinfo, password)
			location = location[at+1:]
		}
		if path != "" {
			secrets = append(secrets, location)
		}
		query, _ := url.ParseQuery(rawQuery)
		for _, values := range query {
			secrets = append(secrets, secretValues(values)...)
		}
	}
	return static(secrets)
}

func secretValues(values []string) []string {
	var secrets []string
	for _, value := range values {
		if _, err := strconv.ParseBool(value); err == nil {
			continue
		}
		if _, err := strconv.ParseFloat(value, 64); err == nil {
			continue
		}
		secrets = append(secrets, value)
	}
	return secrets
}

type Writer struct {
	out      io.Writer
	redactor *Redactor
}

func NewWriter(out io.Writer, redactor *Redactor) *Writer {
	return &Writer{out: out, redactor: redactor}
}

func (w *Writer) Write(data []byte) (int, error) {
	if _, err := io.WriteString(w.out, w.redactor.Redact(string(data))); err != nil {
		return 0, err
	}
	return len(data), nil
}
