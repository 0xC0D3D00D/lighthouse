package record

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/lotusdblabs/lotusdb/v2"

	"github.com/0xc0d3d00d/lighthouse/internal/recordbook/model"
)

var ErrNotFound = errors.New("record not found")

// Repository persists records in lotusdb keyed by "name|qtype".
type Repository struct {
	db    *lotusdb.DB
	count atomic.Int64
}

func NewRepository(path string) (*Repository, error) {
	opts := lotusdb.DefaultOptions
	opts.DirPath = path
	db, err := lotusdb.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open lotusdb at %s: %w", path, err)	}
	r := &Repository{db: db}
	if err := r.initCount(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return r, nil
}

func (r *Repository) initCount() error {
	it, err := r.db.NewIterator(lotusdb.IteratorOptions{})
	if err != nil {
		return fmt.Errorf("count records: %w", err)
	}
	defer it.Close()
	var n int64
	for it.Rewind(); it.Valid(); it.Next() {
		n++
	}
	r.count.Store(n)
	return nil
}

func (r *Repository) Put(rec model.Record) error {
	val, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	key := []byte(rec.Key())
	existed, err := r.db.Exist(key)
	if err != nil {
		return err
	}
	if err := r.db.Put(key, val); err != nil {
		return err
	}
	if !existed {
		r.count.Add(1)
	}
	return nil
}

func (r *Repository) Get(name string, qtype uint16) (model.Record, error) {
	val, err := r.db.Get([]byte(model.Key(name, qtype)))
	if err != nil {
		if errors.Is(err, lotusdb.ErrKeyNotFound) {
			return model.Record{}, ErrNotFound
		}
		return model.Record{}, err
	}
	var rec model.Record
	if err := json.Unmarshal(val, &rec); err != nil {
		return model.Record{}, err
	}
	return rec, nil
}

func (r *Repository) Delete(name string, qtype uint16) error {
	key := []byte(model.Key(name, qtype))
	existed, err := r.db.Exist(key)
	if err != nil {
		return err
	}
	if !existed {
		return ErrNotFound
	}
	if err := r.db.Delete(key); err != nil {
		return err
	}
	r.count.Add(-1)
	return nil
}

// List returns records whose name starts with prefix, optionally filtered by
// qtype (0 = any), skipping offset matches and returning at most limit records.
// It also reports the total number of matches.
func (r *Repository) List(prefix string, qtype uint16, offset, limit int) ([]model.Record, int, error) {
	it, err := r.db.NewIterator(lotusdb.IteratorOptions{Prefix: []byte(prefix)})
	if err != nil {
		return nil, 0, err
	}
	defer it.Close()

	var (
		out   []model.Record
		total int
	)
	for it.Rewind(); it.Valid(); it.Next() {
		var rec model.Record
		if err := json.Unmarshal(it.Value(), &rec); err != nil {
			continue
		}
		if qtype != 0 && rec.Qtype != qtype {
			continue
		}
		if prefix != "" && !strings.HasPrefix(rec.Name, prefix) {
			continue
		}
		if total >= offset && len(out) < limit {
			out = append(out, rec)
		}
		total++
	}
	return out, total, nil
}

func (r *Repository) Count() int64 {
	return r.count.Load()
}

func (r *Repository) Close() error {
	return r.db.Close()
}
