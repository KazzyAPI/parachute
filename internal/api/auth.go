package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

type tokenMatch struct {
	Admin  bool
	Status bool
}

type tokenSet struct {
	key       []byte
	admin     []byte
	status    []byte
	hasStatus bool
	compare   func(x, y []byte) int
}

func newTokenSet(key []byte, admin, status string) *tokenSet {
	tokens := &tokenSet{key: key, hasStatus: status != "", compare: subtle.ConstantTimeCompare}
	tokens.admin, tokens.status = tokens.digest(admin), tokens.digest(status)
	return tokens
}

func (t *tokenSet) digest(token string) []byte {
	mac := hmac.New(sha256.New, t.key)
	mac.Write([]byte(token))
	return mac.Sum(nil)
}

func (t *tokenSet) match(presented string) tokenMatch {
	digest := t.digest(presented)
	admin := t.compare(digest, t.admin) == 1
	status := t.compare(digest, t.status) == 1
	return tokenMatch{Admin: admin, Status: status && t.hasStatus}
}

func (t *tokenSet) allows(r *http.Request, statusEndpoint bool) bool {
	if scheme, presented, found := strings.Cut(r.Header.Get("Authorization"), " "); found && strings.EqualFold(scheme, "Bearer") {
		match := t.match(presented)
		return match.Admin || statusEndpoint && match.Status
	}
	if _, password, ok := r.BasicAuth(); ok && statusEndpoint {
		match := t.match(password)
		return match.Admin || match.Status
	}
	return false
}
