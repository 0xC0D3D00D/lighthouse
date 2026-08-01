package model

import (
	"fmt"
	"time"
)

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
}

// Key is the storage key for a (name, qtype) pair.
func (r Record) Key() string {
	return Key(r.Name, r.Qtype)
}

func Key(name string, qtype uint16) string {
	return fmt.Sprintf("%s|%d", name, qtype)
}
