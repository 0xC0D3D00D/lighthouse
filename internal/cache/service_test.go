package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetGetDelete(t *testing.T) {
	s, err := New(100)
	require.NoError(t, err)
	defer s.Close()

	packed := []byte{1, 2, 3}
	s.Set("example.com.", 1, packed, time.Minute)

	got, ok := s.Get("example.com.", 1)
	require.True(t, ok, "entry not found after Set")
	assert.Equal(t, packed, got)

	_, ok = s.Get("example.com.", 28)
	assert.False(t, ok, "different qtype must not hit")

	s.Delete("example.com.", 1)
	_, ok = s.Get("example.com.", 1)
	assert.False(t, ok, "entry found after Delete")
}

func TestZeroTTLNotStored(t *testing.T) {
	s, err := New(100)
	require.NoError(t, err)
	defer s.Close()

	s.Set("k.", 1, []byte{1}, 0)
	_, ok := s.Get("k.", 1)
	assert.False(t, ok, "zero-TTL entry should not be cached")
}
