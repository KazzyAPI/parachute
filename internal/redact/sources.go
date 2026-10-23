package redact

import (
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
			secrets = append(secrets, withDecoded(authority[:at])...)
			_, password, _ := strings.Cut(authority[:at], ":")
			secrets = append(secrets, withDecoded(password)...)
			authority, location = authority[at+1:], location[at+1:]
		}
		if !strings.ContainsAny(authority, ".:") {
			secrets = append(secrets, authority)
		}
		if path != "" {
			secrets = append(secrets, location)
			secrets = append(secrets, strings.Split(path, "/")...)
		}
		secrets = append(secrets, querySecrets(rawQuery)...)
	}
	return static(secrets)
}

func querySecrets(rawQuery string) []string {
	var secrets []string
	for pair := range strings.SplitSeq(rawQuery, "&") {
		key, value, _ := strings.Cut(pair, "=")
		if !publicQueryValue(key, value) {
			secrets = append(secrets, withDecoded(value)...)
		}
	}
	return secrets
}

func publicQueryValue(key, value string) bool {
	switch strings.ToLower(key) {
	case "title", "username", "color", "priority", "avatar", "contenttype", "template", "disabletls", "port":
		return true
	}
	if _, err := strconv.ParseBool(value); err == nil {
		return true
	}
	return len(value) <= 6 && strings.Trim(value, "0123456789") == ""
}

func withDecoded(raw string) []string {
	if decoded, err := url.QueryUnescape(raw); err == nil && decoded != raw {
		return []string{raw, decoded}
	}
	return []string{raw}
}
