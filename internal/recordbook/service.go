package recordbook

import (
	"errors"
	"time"

	"github.com/0xc0d3d00d/lighthouse/internal/recordbook/model"
	"github.com/0xc0d3d00d/lighthouse/internal/recordbook/storage/record"
)

var ErrNotFound = record.ErrNotFound

type Service struct {
	repo *record.Repository
}

func New(path string) (*Service, error) {
	repo, err := record.NewRepository(path)
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo}, nil
}

// Save persists a backend answer along with its (informational) TTL and the
// query date.
func (s *Service) Save(name string, qtype uint16, packed []byte, ttl time.Duration, rcode uint16, answers int) error {
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

// LookupPacked returns the stored packed DNS response for (name, qtype).
func (s *Service) LookupPacked(name string, qtype uint16) ([]byte, bool) {
	rec, err := s.repo.Get(name, qtype)
	if err != nil {
		return nil, false
	}
	return rec.Packed, true
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
