package server

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestViewStoreIncrementAndPersist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "views.json")

	v := newViewStore(path)
	if got := v.Increment("/post/a"); got != 1 {
		t.Errorf("first increment = %d, want 1", got)
	}
	v.Increment("/post/a")
	v.Increment("/post/b")
	if got := v.Get("/post/a"); got != 2 {
		t.Errorf("Get(/post/a) = %d, want 2", got)
	}
	if got := v.Get("/post/b"); got != 1 {
		t.Errorf("Get(/post/b) = %d, want 1", got)
	}
	if got := v.Get("/post/missing"); got != 0 {
		t.Errorf("Get(missing) = %d, want 0", got)
	}

	// Flush, then load into a fresh store and verify persistence round-trip.
	if err := v.Flush(); err != nil {
		t.Fatal(err)
	}
	v2 := newViewStore(path)
	if err := v2.load(); err != nil {
		t.Fatal(err)
	}
	if got := v2.Get("/post/a"); got != 2 {
		t.Errorf("after reload, Get(/post/a) = %d, want 2", got)
	}
	if got := v2.Get("/post/b"); got != 1 {
		t.Errorf("after reload, Get(/post/b) = %d, want 1", got)
	}
}

func TestViewStoreFlushSkipsCleanState(t *testing.T) {
	v := newViewStore(filepath.Join(t.TempDir(), "views.json"))
	v.Increment("/x")
	if err := v.Flush(); err != nil {
		t.Fatal(err)
	}
	// Second flush should be a no-op (no file write needed).
	if err := v.Flush(); err != nil {
		t.Errorf("second flush errored: %v", err)
	}
}

func TestViewStoreConcurrentIncrement(t *testing.T) {
	v := newViewStore(filepath.Join(t.TempDir(), "views.json"))
	const workers = 50
	const each = 100

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < each; j++ {
				v.Increment("/post/hot")
			}
		}()
	}
	wg.Wait()

	if got := v.Get("/post/hot"); got != int64(workers*each) {
		t.Errorf("concurrent increment lost updates: got %d, want %d", got, workers*each)
	}
}

func TestViewStoreLoadHandlesMissingFile(t *testing.T) {
	v := newViewStore(filepath.Join(t.TempDir(), "does-not-exist.json"))
	err := v.load()
	if err == nil {
		t.Errorf("expected NotExist error, got nil")
	}
}
