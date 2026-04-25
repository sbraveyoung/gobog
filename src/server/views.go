package server

import (
	"encoding/json"
	"os"
	pathpkg "path"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SmartBrave/gobog/src/config"
	"github.com/astaxie/beego/logs"
)

// viewStore is a per-URL hit counter persisted as a small JSON file. Reads
// are lock-free (atomic.LoadInt64 via a sync.Map of *int64); writes go
// through Increment which uses atomic.AddInt64 on a per-URL counter.
//
// Persistence is best-effort: every persistInterval we serialise the whole
// map to a temp file and atomic-rename it into place, so a crash mid-flush
// either keeps the previous file or installs a fully-formed new one.
type viewStore struct {
	path     string
	counters sync.Map // url string -> *int64
	dirty    int32    // 0 = clean, 1 = pending flush

	// stopFlush signals the background goroutine to exit; set when the
	// store is built with a non-zero persist interval.
	stopFlush chan struct{}
	wg        sync.WaitGroup
}

const persistInterval = 30 * time.Second

var views *viewStore

// initViewStore opens the persisted counter file (creating the dir / file
// if missing) and starts a background flusher. Safe to call multiple times
// — the second call is a no-op.
func initViewStore() {
	if views != nil {
		return
	}
	dir := config.C.Data.Dir
	if dir == "" {
		dir = "./gobog-data"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logs.Warn("views: mkdir failed:", err)
		return
	}
	v := newViewStore(filepath.Join(dir, "views.json"))
	if err := v.load(); err != nil && !os.IsNotExist(err) {
		logs.Warn("views: load failed:", err)
	}
	v.startFlusher(persistInterval)
	views = v
}

// newViewStore is exposed for tests so they can use a temp path without
// touching config.C.
func newViewStore(path string) *viewStore {
	return &viewStore{path: path, stopFlush: make(chan struct{})}
}

// Increment bumps the counter for url and returns the new value.
func (v *viewStore) Increment(url string) int64 {
	if v == nil || url == "" {
		return 0
	}
	cnt, _ := v.counters.LoadOrStore(url, new(int64))
	n := atomic.AddInt64(cnt.(*int64), 1)
	atomic.StoreInt32(&v.dirty, 1)
	return n
}

// Get returns the current count for url (0 if unseen).
func (v *viewStore) Get(url string) int64 {
	if v == nil {
		return 0
	}
	cnt, ok := v.counters.Load(url)
	if !ok {
		return 0
	}
	return atomic.LoadInt64(cnt.(*int64))
}

func (v *viewStore) snapshot() map[string]int64 {
	out := make(map[string]int64)
	v.counters.Range(func(k, val interface{}) bool {
		out[k.(string)] = atomic.LoadInt64(val.(*int64))
		return true
	})
	return out
}

// Flush writes the current state to disk via a temp-file + rename so a
// concurrent crash never leaves a half-written file. Idempotent if the
// state is clean since the last flush.
func (v *viewStore) Flush() error {
	if atomic.LoadInt32(&v.dirty) == 0 {
		return nil
	}
	atomic.StoreInt32(&v.dirty, 0)

	data, err := json.MarshalIndent(v.snapshot(), "", "  ")
	if err != nil {
		atomic.StoreInt32(&v.dirty, 1)
		return err
	}
	tmp := v.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(v.path), 0o755); err != nil {
		atomic.StoreInt32(&v.dirty, 1)
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		atomic.StoreInt32(&v.dirty, 1)
		return err
	}
	if err := os.Rename(tmp, v.path); err != nil {
		atomic.StoreInt32(&v.dirty, 1)
		return err
	}
	return nil
}

func (v *viewStore) load() error {
	data, err := os.ReadFile(v.path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var m map[string]int64
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	for k, n := range m {
		x := n
		v.counters.Store(k, &x)
	}
	return nil
}

func (v *viewStore) startFlusher(interval time.Duration) {
	v.wg.Add(1)
	go func() {
		defer v.wg.Done()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := v.Flush(); err != nil {
					logs.Warn("views: flush:", err)
				}
			case <-v.stopFlush:
				_ = v.Flush()
				return
			}
		}
	}()
}

// stop is exposed for tests so they can shut the flusher down deterministically.
func (v *viewStore) stop() {
	if v == nil {
		return
	}
	close(v.stopFlush)
	v.wg.Wait()
}

// pathpkg keeps the import warning at bay if filepath ever goes unused;
// remove if you refactor the package to use only filepath.
var _ = pathpkg.Join
