package recordbook

import (
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/0xc0d3d00d/lighthouse/internal/recordbook/model"
	"github.com/0xc0d3d00d/lighthouse/internal/recordbook/storage/record"
)

var (
	ErrNotFound     = record.ErrNotFound
	ErrManualRecord = model.ErrManualRecord
)

// lockStripes is the number of striped key mutexes serializing writes.
const lockStripes = 64

type Service struct {
	repo *record.Repository
	// locks serialize writes per key (striped) so a save is an atomic
	// check-then-write: a manual save cannot interleave with a backend save,
	// and backend saves racing a manual save are rejected once it lands.
	locks [lockStripes]sync.Mutex
}

func New(path string) (*Service, error) {
	repo, err := record.NewRepository(path)
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo}, nil
}

func (s *Service) keyLock(name string, qtype uint16) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(model.Key(name, qtype)))
	return &s.locks[h.Sum32()%lockStripes]
}

// Save persists a backend answer along with its (informational) TTL and the
// query date. It is rejected with ErrManualRecord if the key holds a manually
// added record, which is the source of truth.
func (s *Service) Save(name string, qtype uint16, packed []byte, ttl time.Duration, rcode uint16, answers int) error {
	mu := s.keyLock(name, qtype)
	mu.Lock()
	defer mu.Unlock()

	existing, err := s.repo.Get(name, qtype)
	if err != nil && !errors.Is(err, record.ErrNotFound) {
		return err
	}
	if err == nil && existing.Manual {
		return fmt.Errorf("save %s/%d: %w", name, qtype, ErrManualRecord)
	}
	return s.repo.Put(model.Record{
		Name:        name,
		Qtype:       qtype,
		Packed:      packed,
		TTL:         uint32(ttl / time.Second),
		QueriedAt:   time.Now(),
		Rcode:       rcode,
		AnswerCount: answers,
	})
}

// SaveManual persists an operator-added record. It unconditionally overwrites
// whatever is stored for the key and marks it manual, atomically with respect
// to concurrent backend saves on the same key.
func (s *Service) SaveManual(name string, qtype uint16, packed []byte, ttl time.Duration, answers int) error {
	mu := s.keyLock(name, qtype)
	mu.Lock()
	defer mu.Unlock()

	return s.repo.Put(model.Record{
		Name:        name,
		Qtype:       qtype,
		Packed:      packed,
		TTL:         uint32(ttl / time.Second),
		QueriedAt:   time.Now(),
		Rcode:       0,
		AnswerCount: answers,
		Manual:      true,
	})
}

// LookupPacked returns the stored packed DNS response for (name, qtype).
func (s *Service) LookupPacked(name string, qtype uint16) ([]byte, bool) {
	rec, err := s.repo.Get(name, qtype)
	if err != nil {
		return nil, false
	}
	return rec.Packed, true
}

// LookupManualPacked returns the packed response and TTL for (name, qtype)
// only when the stored record was added manually.
func (s *Service) LookupManualPacked(name string, qtype uint16) ([]byte, time.Duration, bool) {
	rec, err := s.repo.Get(name, qtype)
	if err != nil || !rec.Manual {
		return nil, 0, false
	}
	return rec.Packed, time.Duration(rec.TTL) * time.Second, true
}

func (s *Service) Lookup(name string, qtype uint16) (model.Record, error) {
	return s.repo.Get(name, qtype)
}

func (s *Service) List(prefix string, qtype uint16, offset, limit int) ([]model.Record, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.List(prefix, qtype, offset, limit)
}

func (s *Service) Delete(name string, qtype uint16) error {
	mu := s.keyLock(name, qtype)
	mu.Lock()
	defer mu.Unlock()

	err := s.repo.Delete(name, qtype)
	if err != nil && !errors.Is(err, record.ErrNotFound) {
		return err
	}
	return err
}

func (s *Service) Count() int64 {
	return s.repo.Count()
}

func (s *Service) Close() error {
	return s.repo.Close()
}
