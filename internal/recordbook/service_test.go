package recordbook

import (
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
	require.NoError(t, s.Save(name, qtype, []byte{0xde, 0xad}, 300*time.Second, 0, 1))
}

func TestSaveLookupDelete(t *testing.T) {
	s := newService(t)

	save(t, s, "example.com.", 1)

	got, err := s.Lookup("example.com.", 1)
	require.NoError(t, err)
	assert.Equal(t, "example.com.", got.Name)
	assert.EqualValues(t, 300, got.TTL)
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
