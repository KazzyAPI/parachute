package redact

import (
	"cmp"
	"encoding/base64"
	"regexp"
	"slices"
	"strings"
)

const (
	placeholder     = "[REDACTED]"
	minimumFreeText = 4
)

type Source interface {
	Secrets() []string
}

type static []string

func (s static) Secrets() []string { return s }

func Static(values ...string) Source { return static(values) }

type Redactor struct {
	sources       []Source
	url           *regexp.Regexp
	queryValue    *regexp.Regexp
	authorization *regexp.Regexp
}

func New(sources ...Source) *Redactor {
	return &Redactor{
		sources:       sources,
		url:           regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s"'<>]+`),
		queryValue:    regexp.MustCompile(`([?&][^=&#?]*=)[^&#]+`),
		authorization: regexp.MustCompile(`(?i)(authorization:\s*(?:(?:bearer|basic)\s+)?)[^\s,;"]+`),
	}
}

func (r *Redactor) Redact(text string) string {
	for _, secret := range r.literals() {
		text = strings.ReplaceAll(text, secret, placeholder)
	}
	text = r.url.ReplaceAllStringFunc(text, r.redactURL)
	return r.authorization.ReplaceAllString(text, "${1}"+placeholder)
}

func (r *Redactor) literals() []string {
	var literals []string
	for _, source := range r.sources {
		for _, secret := range source.Secrets() {
			if len(secret) < minimumFreeText {
				continue
			}
			raw := []byte(secret)
			literals = append(literals, secret,
				base64.StdEncoding.EncodeToString(raw), base64.RawStdEncoding.EncodeToString(raw),
				base64.URLEncoding.EncodeToString(raw), base64.RawURLEncoding.EncodeToString(raw))
		}
	}
	slices.SortFunc(literals, func(left, right string) int {
		return cmp.Or(cmp.Compare(len(right), len(left)), cmp.Compare(left, right))
	})
	return slices.Compact(literals)
}

func (r *Redactor) redactURL(address string) string {
	scheme, rest, _ := strings.Cut(address, "://")
	authorityEnd := strings.IndexAny(rest, "/?#")
	if authorityEnd < 0 {
		authorityEnd = len(rest)
	}
	if at := strings.LastIndex(rest[:authorityEnd], "@"); at >= 0 {
		rest = placeholder + rest[at:]
	}
	if query := strings.Index(rest, "?"); query >= 0 {
		rest = rest[:query] + r.queryValue.ReplaceAllString(rest[query:], "${1}"+placeholder)
	}
	return scheme + "://" + rest
}
