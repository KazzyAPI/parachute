package backup

import (
	"context"
	"fmt"
	"strings"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/moby/moby/api/types/mount"
)

const smallOutput = 1 << 20

type runner struct {
	engine Engine
	image  string
	txn    string
}

func (r *runner) spec(mounts []mount.Mount, command []string) engine.HelperSpec {
	labels := map[string]string{"parachute.helper": "true"}
	if r.txn != "" {
		labels["parachute.transaction"] = r.txn
	}
	return engine.HelperSpec{
		Image: r.image, Command: command, Mounts: mounts, Labels: labels,
		CapAdd: []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID"},
	}
}

func (r *runner) run(ctx context.Context, mounts []mount.Mount, limit int64, script string, args ...string) ([]byte, error) {
	command := append([]string{"sh", "-c", script, "sh"}, args...)
	id, err := r.engine.RunHelper(ctx, r.spec(mounts, command))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.engine.Remove(context.WithoutCancel(ctx), id, false, true) }()
	code, err := r.engine.Wait(ctx, id)
	if err != nil {
		return nil, err
	}
	out, err := r.engine.Logs(ctx, id, limit)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("helper exit %d: %s", code, strings.TrimSpace(string(out[:min(len(out), 512)])))
	}
	return out, nil
}

type HelperFS struct {
	r *runner
}

const resolveScript = `s=/; cur=""; rest="${1#/}"; links=0
while [ -n "$rest" ]; do
  comp="${rest%%"$s"*}"
  case "$rest" in *"$s"*) rest="${rest#*"$s"}";; *) rest="";; esac
  case "$comp" in ""|.) continue;; ..) cur="${cur%"$s"*}"; continue;; esac
  next="$cur/$comp"
  if [ -L "/host$next" ]; then
    links=$((links+1)); [ "$links" -gt 40 ] && exit 1
    t="$(readlink "/host$next")"
    case "$t" in "$s"*) cur=""; rest="${t#/}${rest:+/$rest}";; *) rest="$t${rest:+/$rest}";; esac
  else
    cur="$next"
  fi
done
[ -n "$cur" ] || cur=/
kind=o
if [ -d "/host$cur" ] || [ -f "/host$cur" ]; then kind=b; fi
echo "$kind $cur"`

func (h HelperFS) Resolve(ctx context.Context, path string) (Resolved, error) {
	host := mount.Mount{Type: mount.TypeBind, Source: "/", Target: "/host", ReadOnly: true}
	out, err := h.r.run(ctx, []mount.Mount{host}, smallOutput, resolveScript, path)
	if err != nil {
		return Resolved{}, err
	}
	kind, resolved, ok := strings.Cut(strings.TrimSpace(string(out)), " ")
	if !ok {
		return Resolved{}, fmt.Errorf("resolve %s: unexpected output", path)
	}
	return Resolved{Path: resolved, Backable: kind == "b"}, nil
}

func (h HelperFS) Size(ctx context.Context, source mount.Mount) (int64, error) {
	source.Target = "/src"
	source.ReadOnly = true
	out, err := h.r.run(ctx, []mount.Mount{source}, smallOutput, `du -sk /src | cut -f1`)
	if err != nil {
		return 0, err
	}
	var kib int64
	if _, err := fmt.Sscan(strings.TrimSpace(string(out)), &kib); err != nil {
		return 0, fmt.Errorf("parse size: %w", err)
	}
	return kib * 1024, nil
}
