package server

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sbraveyoung/gobog/src/config"
)

func makePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Solid blue so a white watermark is visible.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{0, 0, 200, 255})
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func makeJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
}

// withWatermarkConfig sets test-scoped config and returns a restore func.
func withWatermarkConfig(t *testing.T, text string) func() {
	t.Helper()
	prevImg := config.C.Image
	prevData := config.C.Data
	config.C.Image = config.ImageConfig{
		WatermarkText:     text,
		WatermarkPosition: "bottom-right",
	}
	config.C.Data = config.DataConfig{Dir: t.TempDir()}
	return func() {
		config.C.Image = prevImg
		config.C.Data = prevData
	}
}

func TestServeWatermarkedDisabledWhenTextEmpty(t *testing.T) {
	cleanup := withWatermarkConfig(t, "")
	defer cleanup()

	src := filepath.Join(t.TempDir(), "x.png")
	makePNG(t, src, 32, 32)

	req := httptest.NewRequest("GET", "/image/x.png", nil)
	w := httptest.NewRecorder()
	if got := serveWatermarked(w, req, src); got {
		t.Errorf("expected serveWatermarked to refuse without watermark text")
	}
}

func TestServeWatermarkedSkipsUnsupportedFormats(t *testing.T) {
	cleanup := withWatermarkConfig(t, "© test")
	defer cleanup()

	src := filepath.Join(t.TempDir(), "x.svg")
	if err := os.WriteFile(src, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/image/x.svg", nil)
	w := httptest.NewRecorder()
	if got := serveWatermarked(w, req, src); got {
		t.Errorf("svg shouldn't be watermarked")
	}
}

func TestServeWatermarkedPNG(t *testing.T) {
	cleanup := withWatermarkConfig(t, "© test")
	defer cleanup()

	src := filepath.Join(t.TempDir(), "x.png")
	makePNG(t, src, 100, 60)

	req := httptest.NewRequest("GET", "/image/x.png", nil)
	w := httptest.NewRecorder()
	if !serveWatermarked(w, req, src) {
		t.Fatal("expected serveWatermarked to handle PNG")
	}
	if got := w.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", got)
	}
	// Decode the response — should still be a valid PNG of the same size.
	out, _, err := image.Decode(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("response not a valid image: %v", err)
	}
	if out.Bounds().Dx() != 100 || out.Bounds().Dy() != 60 {
		t.Errorf("dimensions changed: %v", out.Bounds())
	}
}

func TestServeWatermarkedCachesOnDisk(t *testing.T) {
	cleanup := withWatermarkConfig(t, "© cache")
	defer cleanup()

	src := filepath.Join(t.TempDir(), "x.png")
	makePNG(t, src, 80, 80)

	req := httptest.NewRequest("GET", "/image/x.png", nil)
	w := httptest.NewRecorder()
	if !serveWatermarked(w, req, src) {
		t.Fatal("first call failed")
	}
	first := w.Body.Bytes()

	// Cache file should now exist under <data>/wm-cache/.
	cacheRoot := filepath.Join(config.C.Data.Dir, "wm-cache")
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		t.Fatalf("cache dir missing: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 cached file, got %d", len(entries))
	}

	// Second call should serve from cache and yield identical bytes.
	w2 := httptest.NewRecorder()
	if !serveWatermarked(w2, req, src) {
		t.Fatal("second call failed")
	}
	if !bytes.Equal(first, w2.Body.Bytes()) {
		t.Errorf("cached bytes differ from first response (cache miss?)")
	}
}

func TestApplyWatermarkPosition(t *testing.T) {
	src := filepath.Join(t.TempDir(), "x.jpg")
	makeJPEG(t, src, 200, 100)
	raw, _ := os.ReadFile(src)
	for _, pos := range []string{"top-left", "top-right", "bottom-left", "bottom-right", "center", ""} {
		out, err := applyWatermark(raw, ".jpg", "© test", pos)
		if err != nil {
			t.Errorf("pos %q: %v", pos, err)
			continue
		}
		if _, _, err := image.Decode(bytes.NewReader(out)); err != nil {
			t.Errorf("pos %q: result not a valid image: %v", pos, err)
		}
	}
}

func TestServeWatermarkedRespondsCorrectStatus(t *testing.T) {
	cleanup := withWatermarkConfig(t, "© code")
	defer cleanup()

	src := filepath.Join(t.TempDir(), "x.png")
	makePNG(t, src, 40, 40)

	req := httptest.NewRequest("GET", "/image/x.png", nil)
	w := httptest.NewRecorder()
	if !serveWatermarked(w, req, src) {
		t.Fatal("watermark should have served")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
