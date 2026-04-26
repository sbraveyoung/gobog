package server

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/SmartBrave/gobog/src/config"
	"github.com/astaxie/beego/logs"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Watermark serves the file at sourcePath as an HTTP response, overlaying
// [image].watermark_text on JPEG / PNG content. Returns false when the file
// can't be watermarked (unsupported format, decode error) so the caller
// can fall back to a plain http.ServeFile.
//
// Watermarked bytes are cached on disk under <data>/wm-cache/. Cache key
// includes the source path + source mtime + watermark text + position so
// edits to the source file (or to the config) invalidate stale entries.
func serveWatermarked(w http.ResponseWriter, r *http.Request, sourcePath string) bool {
	if !watermarkEnabled() {
		return false
	}
	ext := strings.ToLower(filepath.Ext(sourcePath))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return false
	}
	fi, err := os.Stat(sourcePath)
	if err != nil {
		return false
	}

	cachePath, err := watermarkCachePath(sourcePath, fi.ModTime().UnixNano(), ext)
	if err != nil {
		logs.Warn("watermark cache path:", err)
		return false
	}

	// Fast path: cached output already exists, just serve it.
	if cfi, err := os.Stat(cachePath); err == nil && cfi.ModTime().After(fi.ModTime()) {
		http.ServeFile(w, r, cachePath)
		return true
	}

	src, err := os.ReadFile(sourcePath)
	if err != nil {
		return false
	}
	out, err := applyWatermark(src, ext, config.C.Image.WatermarkText, config.C.Image.WatermarkPosition)
	if err != nil {
		logs.Warn("watermark:", err)
		return false
	}

	// Best-effort cache write: failures don't break the response.
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
		_ = os.WriteFile(cachePath, out, 0o644)
	}

	if ext == ".png" {
		w.Header().Set("Content-Type", "image/png")
	} else {
		w.Header().Set("Content-Type", "image/jpeg")
	}
	_, _ = io.Copy(w, bytes.NewReader(out))
	return true
}

func watermarkEnabled() bool {
	return strings.TrimSpace(config.C.Image.WatermarkText) != ""
}

var watermarkKeyMu sync.Mutex

func watermarkCachePath(sourcePath string, mtime int64, ext string) (string, error) {
	dir := config.C.Data.Dir
	if dir == "" {
		dir = "./gobog-data"
	}
	watermarkKeyMu.Lock()
	defer watermarkKeyMu.Unlock()
	h := sha1.New()
	fmt.Fprintf(h, "%s\x00%d\x00%s\x00%s",
		sourcePath, mtime,
		config.C.Image.WatermarkText,
		config.C.Image.WatermarkPosition,
	)
	id := hex.EncodeToString(h.Sum(nil))
	return filepath.Join(dir, "wm-cache", id+ext), nil
}

// applyWatermark decodes image bytes, draws the text in white-with-shadow at
// the chosen corner, and re-encodes. Position values: "top-left",
// "top-right", "bottom-left", "bottom-right" (default), "center".
func applyWatermark(data []byte, ext, text, position string) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	bounds := src.Bounds()
	canvas := image.NewRGBA(bounds)
	draw.Draw(canvas, bounds, src, bounds.Min, draw.Src)

	face := basicfont.Face7x13
	x, y := watermarkAnchor(bounds, text, face, position)

	// Shadow (1px black offset) for legibility on bright backgrounds.
	drawString(canvas, x+1, y+1, color.RGBA{0, 0, 0, 200}, face, text)
	drawString(canvas, x, y, color.RGBA{255, 255, 255, 220}, face, text)

	var out bytes.Buffer
	switch ext {
	case ".png":
		if err := png.Encode(&out, canvas); err != nil {
			return nil, err
		}
	default:
		if err := jpeg.Encode(&out, canvas, &jpeg.Options{Quality: 88}); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func drawString(dst draw.Image, x, y int, c color.Color, face font.Face, text string) {
	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(text)
}

func watermarkAnchor(bounds image.Rectangle, text string, face font.Face, position string) (int, int) {
	const margin = 8
	textWidth := font.MeasureString(face, text).Round()
	textHeight := face.Metrics().Ascent.Round()

	switch strings.ToLower(strings.TrimSpace(position)) {
	case "top-left":
		return bounds.Min.X + margin, bounds.Min.Y + margin + textHeight
	case "top-right":
		return bounds.Max.X - textWidth - margin, bounds.Min.Y + margin + textHeight
	case "bottom-left":
		return bounds.Min.X + margin, bounds.Max.Y - margin
	case "center":
		return bounds.Min.X + (bounds.Dx()-textWidth)/2, bounds.Min.Y + (bounds.Dy()+textHeight)/2
	default: // bottom-right
		return bounds.Max.X - textWidth - margin, bounds.Max.Y - margin
	}
}
