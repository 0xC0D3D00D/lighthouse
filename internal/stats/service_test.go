package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRecordAndSnapshot(t *testing.T) {
	s := New(time.Minute)
	s.RecordQuery(false)
	s.RecordQuery(true)
	s.RecordError()

	snap := s.Snapshot(10)
	assert.EqualValues(t, 2, snap.TotalQueries)
	assert.EqualValues(t, 1, snap.TotalCacheHits)
	assert.EqualValues(t, 1, snap.TotalErrors)
	assert.Len(t, snap.Buckets, 10)

	var q, hits, errs int64
	for _, b := range snap.Buckets {
		q += b.Queries
		hits += b.CacheHits
		errs += b.Errors
	}
	assert.EqualValues(t, 2, q)
	assert.EqualValues(t, 1, hits)
	assert.EqualValues(t, 1, errs)
}

func TestSnapshotClampsWindow(t *testing.T) {
	s := New(time.Minute)
	assert.Len(t, s.Snapshot(0).Buckets, 60)
	assert.Len(t, s.Snapshot(100000).Buckets, 60)
}
