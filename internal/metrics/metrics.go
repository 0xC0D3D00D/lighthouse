package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Frontend (client-facing) metrics.
	FrontendQueries = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "lighthouse",
		Subsystem: "frontend",
		Name:      "queries_total",
		Help:      "DNS queries received from clients.",
	}, []string{"proto"})
	FrontendErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "lighthouse",
		Subsystem: "frontend",
		Name:      "errors_total",
		Help:      "DNS queries that could not be answered (servfail/empty).",
	}, []string{"proto"})
	CacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "lighthouse",
		Subsystem: "frontend",
		Name:      "cache_hits_total",
		Help:      "Queries answered from the in-memory cache.",
	})

	// Backend (backend-facing) metrics.
	BackendQueries = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "lighthouse",
		Subsystem: "backend",
		Name:      "queries_total",
		Help:      "Queries sent to backend servers.",
	}, []string{"backend"})
	BackendErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "lighthouse",
		Subsystem: "backend",
		Name:      "errors_total",
		Help:      "Failed queries to backend servers.",
	}, []string{"backend"})

	SurvivalMode = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "lighthouse",
		Name:      "survival_mode",
		Help:      "1 when the server is in survival mode, 0 otherwise.",
	})
)

// RegisterCacheSize exposes the number of entries in the in-memory cache.
func RegisterCacheSize(f func() float64) {
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "lighthouse",
		Subsystem: "cache",
		Name:      "entries",
		Help:      "Records currently stored in the otter in-memory cache.",
	}, f)
}

// RegisterRecordBookSize exposes the number of records in the lotusdb record book.
func RegisterRecordBookSize(f func() float64) {
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "lighthouse",
		Subsystem: "recordbook",
		Name:      "records",
		Help:      "Records currently stored in the lotusdb record book.",
	}, f)
}
