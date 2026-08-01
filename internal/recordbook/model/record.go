package model

import (
	"errors"
	"fmt"
	"time"
)

// ErrManualRecord is returned when a resolver write is rejected because the
// stored record was added manually and is the source of truth for its key.
var ErrManualRecord = errors.New("record is manually pinned")

// Record is a persisted DNS query result.
type Record struct {
	// Name is the fully qualified query name.
	Name string `json:"name"`
	// Qtype is the numeric DNS record type.
	Qtype uint16 `json:"qtype"`
	// Packed is the full packed DNS response message (wire format).
	Packed []byte `json:"packed"`
	// TTL is the smallest answer TTL in seconds at query time. Informational only;
	// records are served from the book regardless of expiry in survival mode.
	TTL uint32 `json:"ttl"`
	// QueriedAt is when the answer was obtained from an backend.
	QueriedAt time.Time `json:"queried_at"`
	// Rcode of the stored response (0 = success).
	Rcode uint16 `json:"rcode"`
	// AnswerCount is the number of answer RRs in the stored response.
	AnswerCount int `json:"answer_count"`
	// Source traces where the answer came from: the backend name (e.g.
	// "udp://1.1.1.1:53") for resolver saves, or "manual" for operator-added
	// records. Empty for records stored before source tracking existed.
	Source string `json:"source"`
	// Manual marks a record added by an operator through the dashboard. Manual
	// records are the source of truth: backend answers never overwrite them and
	// they are served ahead of backend queries.
	Manual bool `json:"manual"`
}

// Key is the storage key for a (name, qtype) pair.
func (r Record) Key() string {
	return Key(r.Name, r.Qtype)
}

func Key(name string, qtype uint16) string {
	return fmt.Sprintf("%s|%d", name, qtype)
}
