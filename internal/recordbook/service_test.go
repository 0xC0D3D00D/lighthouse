package recordbook

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newService(t *testing.T) *Service {
	t.Helper()
	s, err := New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func save(t *testing.T, s *Service, name string, qtype uint16) {
	t.Helper()
	require.NoError(t, s.Save(name, qtype, []byte{0xde, 0xad}, 300*time.Second, 0, 1, "udp://1.1.1.1:53"))
}

func TestSaveLookupDelete(t *testing.T) {
	s := newService(t)

	save(t, s, "example.com.", 1)

	got, err := s.Lookup("example.com.", 1)
	require.NoError(t, err)
	assert.Equal(t, "example.com.", got.Name)
	assert.EqualValues(t, 300, got.TTL)
	assert.Equal(t, "udp://1.1.1.1:53", got.Source)
	assert.EqualValues(t, 1, s.Count())

	packed, ok := s.LookupPacked("example.com.", 1)
	require.True(t, ok)
	assert.Equal(t, []byte{0xde, 0xad}, packed)

	require.NoError(t, s.Delete("example.com.", 1))
	_, err = s.Lookup("example.com.", 1)
	assert.ErrorIs(t, err, ErrNotFound)
	_, ok = s.LookupPacked("example.com.", 1)
	assert.False(t, ok)
	assert.EqualValues(t, 0, s.Count())
}

func TestSaveOverwriteKeepsCount(t *testing.T) {
	s := newService(t)
	save(t, s, "a.example.", 1)
	save(t, s, "a.example.", 1)
	assert.EqualValues(t, 1, s.Count())
}

func TestSaveManualOverwritesAndPins(t *testing.T) {
	s := newService(t)
	save(t, s, "pin.example.", 1)

	require.NoError(t, s.SaveManual("pin.example.", 1, []byte{0xbe, 0xef}, 600*time.Second, 1))
	assert.EqualValues(t, 1, s.Count())

	got, err := s.Lookup("pin.example.", 1)
	require.NoError(t, err)
	assert.True(t, got.Manual)
	assert.Equal(t, "manual", got.Source)
	assert.Equal(t, []byte{0xbe, 0xef}, got.Packed)
	assert.EqualValues(t, 600, got.TTL)

	// Backend saves are rejected once the record is manual.
	err = s.Save("pin.example.", 1, []byte{0xde, 0xad}, 300*time.Second, 0, 1, "udp://1.1.1.1:53")
	assert.ErrorIs(t, err, ErrManualRecord)
	got, err = s.Lookup("pin.example.", 1)
	require.NoError(t, err)
	assert.Equal(t, []byte{0xbe, 0xef}, got.Packed)

	packed, ttl, ok := s.LookupManualPacked("pin.example.", 1)
	require.True(t, ok)
	assert.Equal(t, []byte{0xbe, 0xef}, packed)
	assert.Equal(t, 600*time.Second, ttl)

	// A later manual save still overwrites.
	require.NoError(t, s.SaveManual("pin.example.", 1, []byte{0xca, 0xfe}, 60*time.Second, 1))
	packed, _, ok = s.LookupManualPacked("pin.example.", 1)
	require.True(t, ok)
	assert.Equal(t, []byte{0xca, 0xfe}, packed)

	// Deleting clears the pin; backend saves work again.
	require.NoError(t, s.Delete("pin.example.", 1))
	_, _, ok = s.LookupManualPacked("pin.example.", 1)
	assert.False(t, ok)
	save(t, s, "pin.example.", 1)
}

func TestLookupManualPackedIgnoresBackendRecords(t *testing.T) {
	s := newService(t)
	save(t, s, "plain.example.", 1)
	_, _, ok := s.LookupManualPacked("plain.example.", 1)
	assert.False(t, ok)
}

func TestManualSaveAtomicUnderConcurrentWrites(t *testing.T) {
	s := newService(t)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 20; j++ {
				_ = s.Save("race.example.", 1, []byte{0xde, 0xad}, 300*time.Second, 0, 1, "udp://1.1.1.1:53")
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		require.NoError(t, s.SaveManual("race.example.", 1, []byte{0xbe, 0xef}, 600*time.Second, 1))
	}()
	close(start)
	wg.Wait()

	// The manual record wins: every backend save that ran after it was rejected.
	got, err := s.Lookup("race.example.", 1)
	require.NoError(t, err)
	assert.True(t, got.Manual)
	assert.Equal(t, []byte{0xbe, 0xef}, got.Packed)
	assert.EqualValues(t, 1, s.Count())
}

func TestListFilterAndPagination(t *testing.T) {
	s := newService(t)
	save(t, s, "a.example.", 1)
	save(t, s, "b.example.", 1)
	save(t, s, "b.example.", 28)
	save(t, s, "c.other.", 1)

	recs, total, err := s.List("b.example.", 0, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, recs, 2)

	recs, total, err = s.List("", 28, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, recs, 1)
	assert.EqualValues(t, 28, recs[0].Qtype)

	recs, total, err = s.List("", 0, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, 4, total)
	assert.Len(t, recs, 2)
}
