package api

import (
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

func ParseTrustedProxies(entries []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(entries))
	for _, entry := range entries {
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}
		address, err := netip.ParseAddr(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid --trusted-proxies entry %q: not an IP address or CIDR range", entry)
		}
		address = address.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(address, address.BitLen()))
	}
	return prefixes, nil
}

type Sources struct {
	trusted []netip.Prefix
}

func (s Sources) trusts(address netip.Addr) bool {
	return slices.ContainsFunc(s.trusted, func(prefix netip.Prefix) bool { return prefix.Contains(address) })
}

func (s Sources) Of(r *http.Request) netip.Addr {
	peerAddress, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	peer := peerAddress.Addr().Unmap()
	if !s.trusts(peer) {
		return peer
	}
	forwarded, ok := forwardedFor(r)
	if !ok {
		return peer
	}
	for index := len(forwarded) - 1; index >= 0; index-- {
		if !s.trusts(forwarded[index]) {
			return forwarded[index]
		}
	}
	return peer
}

func forwardedFor(r *http.Request) ([]netip.Addr, bool) {
	var addresses []netip.Addr
	for _, value := range r.Header.Values("X-Forwarded-For") {
		for entry := range strings.SplitSeq(value, ",") {
			address, err := netip.ParseAddr(strings.TrimSpace(entry))
			if err != nil {
				return nil, false
			}
			addresses = append(addresses, address.Unmap())
		}
	}
	return addresses, len(addresses) > 0
}
