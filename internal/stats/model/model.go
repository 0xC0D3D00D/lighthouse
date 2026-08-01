package model

// Bucket aggregates counters for one second.
type Bucket struct {
	Unix           int64 `json:"unix"`
	Queries        int64 `json:"queries"`
	Errors         int64 `json:"errors"`
	CacheHits      int64 `json:"cache_hits"`
	BackendErrors int64 `json:"backend_errors"`
}

// Snapshot is a rolling window of per-second buckets plus running totals.
type Snapshot struct {
	Buckets        []Bucket `json:"buckets"`
	TotalQueries   int64    `json:"total_queries"`
	TotalErrors    int64    `json:"total_errors"`
	TotalCacheHits int64    `json:"total_cache_hits"`
}
