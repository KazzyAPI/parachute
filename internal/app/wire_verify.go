package app

import (
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/txn"
	"github.com/KazzyAPI/parachute/internal/verify"
)

func (c *Container) verifier(docker *engine.Docker) txn.Verifier {
	settings := verify.Settings{Timeout: c.cfg.Verify.Timeout, Stable: c.cfg.Verify.Stable}
	labels := verify.NewLabelAdapter(c.labels)
	var prober verify.URLProber
	if c.cfg.Backup.Helper != "" {
		prober = &urlProber{docker: docker, helperImage: c.cfg.Backup.Helper, ownerLabel: "parachute.transaction"}
	}
	return verify.New(docker, docker, prober, labels, settings, c.deps.Clock)
}
