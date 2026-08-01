package model

import (
	"strings"
	"time"
)

// Backend is a resolver-level view of an backend DNS server.
type Backend struct {
	// Scheme is one of udp, tcp, tls, https.
	Scheme string
	// Address is host:port (udp/tcp/tls).
	Address string
	// URL is the DoH endpoint (https).
	URL string
	// Suffix is the canonical fully-qualified lowercase name suffix this
	// backend serves; "." matches every name.
	Suffix string
}

// Matches reports whether the backend serves the given canonical FQDN.
func (u Backend) Matches(name string) bool {
	if u.Suffix == "" || u.Suffix == "." {
		return true
	}
	name = strings.ToLower(name)
	return name == u.Suffix || strings.HasSuffix(name, "."+u.Suffix)
}

func (u Backend) Name() string {
	if u.Scheme == "https" {
		return u.URL
	}
	return u.Scheme + "://" + u.Address
}

// Result is the outcome of resolving a query.
type Result struct {
	// Packed is the full packed DNS response (wire format), ready to be
	// rewritten with the client's message ID.
	Packed []byte
	// Rcode of the response.
	Rcode uint16
	// AnswerCount is the number of answer RRs.
	AnswerCount int
	// TTL is the smallest TTL in the answer section.
	TTL time.Duration
	// Source describes where the answer came from: cache, backend name, or recordbook.
	Source string
	// Stale is true when served from the record book past its TTL (survival mode).
	Stale bool
}
