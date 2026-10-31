package restoreverify

import (
	"github.com/KazzyAPI/parachute/internal/txn"
)

func ThrowawayContainerName(suffix, original string) string {
	return "parachute-rv-" + suffix + "-" + original
}

func ThrowawayNetworkName(suffix string) string {
	return "parachute-rv-" + suffix
}

func ThrowawayVolumeName(suffix, container, destination string) string {
	return "parachute-rv-" + suffix + "-" + container + "-" + sanitizeMount(destination)
}

func Suffix(runID string) string { return txn.RunSuffix(runID) }

func sanitizeMount(destination string) string {
	var out []byte
	for i := 0; i < len(destination); i++ {
		ch := destination[i]
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9', ch == '.', ch == '_', ch == '-':
			out = append(out, ch)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "root"
	}
	return string(out)
}
