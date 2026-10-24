package registry

import (
	"regexp"
	"strconv"
)

var versionTag = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

type version [3]int

func parseVersion(tag string) (version, bool) {
	match := versionTag.FindStringSubmatch(tag)
	if match == nil {
		return version{}, false
	}
	var parsed version
	for index := range parsed {
		number, err := strconv.Atoi(match[index+1])
		if err != nil {
			return version{}, false
		}
		parsed[index] = number
	}
	return parsed, true
}

func (v version) after(other version) bool {
	for index := range v {
		if v[index] != other[index] {
			return v[index] > other[index]
		}
	}
	return false
}

func FollowTag(current, mode string, tags []string) (string, bool) {
	base, ok := parseVersion(current)
	if !ok || mode != "patch" && mode != "minor" {
		return "", false
	}
	best, found := current, false
	bestVersion := base
	for _, tag := range tags {
		candidate, ok := parseVersion(tag)
		if !ok || candidate[0] != base[0] || mode == "patch" && candidate[1] != base[1] || !candidate.after(bestVersion) {
			continue
		}
		best, bestVersion, found = tag, candidate, true
	}
	return best, found
}

func IsVersionTag(tag string) bool {
	_, ok := parseVersion(tag)
	return ok
}
