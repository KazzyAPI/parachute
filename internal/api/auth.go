package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

type Match struct {
	Admin  bool
	Status bool
}

type Tokens struct {
	admin     [sha256.Size]byte
	status    [sha256.Size]byte
	hasStatus bool
	compare   func(x, y []byte) int
}

func NewTokens(admin, status string) *Tokens {
	return &Tokens{
		admin: sha256.Sum256([]byte(admin)), status: sha256.Sum256([]byte(status)), hasStatus: status != "",
		compare: subtle.ConstantTimeCompare,
	}
}

func (t *Tokens) Match(presented string) Match {
	digest := sha256.Sum256([]byte(presented))
	admin := t.compare(digest[:], t.admin[:]) == 1
	status := t.compare(digest[:], t.status[:]) == 1
	return Match{Admin: admin, Status: status && t.hasStatus}
}

func (t *Tokens) Allows(r *http.Request, statusEndpoint bool) bool {
	if scheme, presented, found := strings.Cut(r.Header.Get("Authorization"), " "); found && strings.EqualFold(scheme, "Bearer") {
		match := t.Match(presented)
		return match.Admin || statusEndpoint && match.Status
	}
	if _, password, ok := r.BasicAuth(); ok && statusEndpoint {
		match := t.Match(password)
		return match.Admin || match.Status
	}
	return false
}
