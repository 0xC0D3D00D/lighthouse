package model

import "time"

type Mode string

const (
	ModeNormal   Mode = "normal"
	ModeSurvival Mode = "survival"
)

// BackendStatus is the latest beacon probe outcome for one backend.
type BackendStatus struct {
	Name      string    `json:"name"`
	Reachable bool      `json:"reachable"`
	CheckedAt time.Time `json:"checked_at"`
}

// Status is a snapshot of the health service state.
type Status struct {
	Mode          Mode             `json:"mode"`
	Backends     []BackendStatus `json:"backends"`
	LastBeaconOK  time.Time        `json:"last_beacon_ok"`
	BeaconOKSince time.Time        `json:"beacon_ok_since"`
	ModeSince     time.Time        `json:"mode_since"`
}
