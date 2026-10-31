package config

import "strings"

type removedFamily struct {
	prefix  string
	example string
}

type RemovedSettings struct {
	families []removedFamily
}

func NewRemovedSettings() *RemovedSettings {
	return &RemovedSettings{families: []removedFamily{
		{prefix: "notification-email-", example: "smtp://user:password@host:587/?from=a@example.com&to=b@example.com"},
		{prefix: "notification-slack-", example: "slack://token@channel"},
		{prefix: "notification-msteams-", example: "teams://group@tenant/altId/groupOwner?host=example.webhook.office.com"},
		{prefix: "notification-gotify-", example: "gotify://host/token"},
	}}
}

func (r *RemovedSettings) Flag(name string) (string, bool) {
	if name == "notifications" {
		return r.replacement("slack://token@channel"), true
	}
	for _, family := range r.families {
		if strings.HasPrefix(name, family.prefix) {
			return r.replacement(family.example), true
		}
	}
	return "", false
}

func (r *RemovedSettings) Env(name string) (string, bool) {
	flag, ok := strings.CutPrefix(name, "PARACHUTE_")
	if !ok {
		return "", false
	}
	return r.Flag(strings.ToLower(strings.ReplaceAll(flag, "_", "-")))
}

func (r *RemovedSettings) Command(name string) (string, bool) {
	if name == "notify-upgrade" {
		return r.replacement("slack://token@channel"), true
	}
	return "", false
}

func (r *RemovedSettings) replacement(example string) string {
	return "use --notification-url with a service URL, for example " + example
}
