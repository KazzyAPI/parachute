package policy

import (
	"strings"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type Databases struct {
	names map[string]bool
}

func NewDatabases() *Databases {
	names := map[string]bool{}
	for _, name := range []string{
		"postgres", "postgresql", "postgis", "timescaledb", "mysql", "mariadb", "percona-server", "mongo", "mongodb",
		"redis", "valkey", "influxdb", "elasticsearch", "opensearch", "cassandra", "couchdb", "clickhouse-server", "neo4j",
	} {
		names[name] = true
	}
	return &Databases{names: names}
}

func (d *Databases) Recognised(reference string) bool {
	return d.names[RepositoryName(reference)]
}

func RepositoryName(reference string) string {
	repository, _, _ := strings.Cut(reference, "@")
	if slash, colon := strings.LastIndex(repository, "/"), strings.LastIndex(repository, ":"); colon > slash {
		repository = repository[:colon]
	}
	return repository[strings.LastIndex(repository, "/")+1:]
}

func (d *Databases) Is(s Subject) bool {
	if database, set := s.Overrides.Database(); set {
		return database
	}
	return d.Recognised(s.Container.ImageRef)
}

type ApprovalRule struct {
	databases *Databases
	protect   bool
}

func NewApprovalRule(databases *Databases, protect bool) *ApprovalRule {
	return &ApprovalRule{databases: databases, protect: protect}
}

func (r *ApprovalRule) Hold(s Subject) (domain.Hold, bool, error) {
	hold := domain.Hold{Reason: reasonApprovalRequired}
	switch s.Overrides.Approval() {
	case "auto":
		return hold, false, nil
	case "required":
		hold.Detail = "parachute.approval=required"
		return hold, !s.Approved, nil
	}
	hold.Detail = "database"
	return hold, r.protect && r.databases.Is(s) && !s.Approved, nil
}
