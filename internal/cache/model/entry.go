package model

import "time"

// Entry is a cached DNS response.
type Entry struct {
	// Packed is the full packed DNS response message (wire format).
	Packed []byte
	// TTL is the smallest TTL found in the answer section, used as the cache lifetime.
	TTL time.Duration
	// StoredAt is when the entry was cached.
	StoredAt time.Time
}
