package article

import (
	"bufio"
	"bytes"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	pathpkg "path"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/astaxie/beego/logs"
)

const (
	ARTICLE = "article"
	DIR     = "dir"
)

const (
	START = iota
	NO_META
	META_BEGIN
	META_END
)

const TIME_LAYOUT = "2006-01-02 15:04:05"
const wordsPerMinute = 250

// Meta is the front-matter struct. All fields stay string-typed because the
// reflective rewriter formats values as plain text.
type Meta struct {
	Title         string `meta:"title"`
	Description   string `meta:"description"`
	Author        string `meta:"author"`
	CreateTime    string `meta:"create_time"`
	Category      string `meta:"category"`
	TagsRaw       string `meta:"tags"`
	Id            string `meta:"id"`
	URL           string `meta:"url"`
	Draft         string `meta:"draft"`
	// Pin sticks an article to the top of its containing listing,
	// overriding the usual create_time descending order.
	Pin string `meta:"pin"`
	// Private articles still appear in indexes but their body is gated
	// by HTTP Basic auth (see config [auth]). Lists show title + URL,
	// no Summary leak.
	Private string `meta:"private"`
	// Hidden articles are dropped from listings entirely (like Draft) but
	// the URL keeps resolving — useful for "temporarily off" rather than
	// "work in progress". Toggle via [blog].include_hidden.
	Hidden string `meta:"hidden"`
	// AI marks an AI-generated / -assisted post. Truthy values (true / 1 /
	// yes / on) render a generic "AI" badge; any other non-empty string
	// is treated as the model name and shown verbatim ("claude", "gpt-4o").
	AI string `meta:"ai"`
	// Cover is an optional hero image — relative path resolved against the
	// blog's image index, or an absolute URL. Renders at the top of the
	// post page and is also used for OG meta.
	Cover         string `meta:"cover"`
	TyporaRootURL string `meta:"typora-root-url"`
}

type Article struct {
	Meta
	Source         string
	Content        []byte
	Parse          string
	Tags           []string
	Summary        string
	WordCount      int
	ReadingTimeMin int
	SubArticle     Articles

	parsedHTML atomic.Value
}

func (a *Article) CachedHTML() (string, bool) {
	if v := a.parsedHTML.Load(); v != nil {
		return v.(string), true
	}
	return "", false
}

func (a *Article) StoreHTML(html string) { a.parsedHTML.Store(html) }

// IsDraft reports whether front-matter marks this article as a draft.
func (a *Article) IsDraft() bool { return truthy(a.Draft) }

// IsPinned sticks the article to the top of its containing listing,
// overriding create_time order.
func (a *Article) IsPinned() bool { return truthy(a.Pin) }

// IsPrivate gates the article body behind HTTP Basic auth.
func (a *Article) IsPrivate() bool { return truthy(a.Private) }

// IsHidden drops the article from listings entirely (URL keeps resolving).
func (a *Article) IsHidden() bool { return truthy(a.Hidden) }

// IsAI reports whether the front-matter marks this article as AI-generated.
// A truthy value (true / 1 / yes / on) counts; any other non-empty,
// non-falsy string is treated as the model name and also counts. Explicit
// false / 0 / no / off return false so authors can write `ai: false` to
// mean "I wrote this myself".
func (a *Article) IsAI() bool {
	v := strings.ToLower(strings.TrimSpace(a.AI))
	if v == "" {
		return false
	}
	switch v {
	case "false", "0", "no", "off":
		return false
	}
	return true
}

// AILabel returns the human-facing badge text. For truthy keywords it
// returns "AI"; otherwise the verbatim trimmed value (so "claude" renders
// as "🤖 claude" while `ai: true` renders as just "🤖 AI"). Returns "" when
// IsAI is false.
func (a *Article) AILabel() string {
	if !a.IsAI() {
		return ""
	}
	v := strings.TrimSpace(a.AI)
	if truthy(v) {
		return "AI"
	}
	return v
}

func (a *Article) IsGroup() bool { return len(a.SubArticle) > 0 }

// truthy interprets common YAML-ish booleans (true / 1 / yes / on, any case)
// as true; everything else (including empty) is false.
func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

type Articles []*Article

func (a Articles) Len() int      { return len(a) }
func (a Articles) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a Articles) Less(i, j int) bool {
	// Groups (directories) before leaves so navigation stays stable.
	if a[i].SubArticle == nil && a[j].SubArticle != nil {
		return false
	}
	if a[i].SubArticle != nil && a[j].SubArticle == nil {
		return true
	}
	// Pinned articles bubble to the top within the same kind.
	if a[i].IsPinned() != a[j].IsPinned() {
		return a[i].IsPinned()
	}
	ti, erri := time.Parse(TIME_LAYOUT, a[i].CreateTime)
	tj, errj := time.Parse(TIME_LAYOUT, a[j].CreateTime)
	if erri != nil || errj != nil {
		return false
	}
	return ti.Unix() > tj.Unix()
}

// ParseFile reads the markdown at path and returns an Article populated from
// front-matter + body. Does NOT touch the file on disk and does NOT default
// missing meta — callers in vault mode supply URL/Id/Title/CreateTime
// derivatively from the path so the user's notes stay untouched.
func ParseFile(path string) (*Article, error) {
	logs.Debug("parse:", path)
	a := &Article{Source: path}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	if err := parseInto(a, bufio.NewReader(f)); err != nil {
		return nil, err
	}

	finalizeDerived(a)
	return a, nil
}

// NewArticle is the legacy constructor. Same shape as before: parses, then
// fills missing meta and rewrites the file in place.
func NewArticle(path, articleType, fatherURL string) (*Article, error) {
	logs.Debug("NewArticle path:", path, " type:", articleType, " father:", fatherURL)
	a := &Article{Source: path}

	if articleType == DIR {
		a.Title = pathpkg.Base(path)
		a.CreateTime = time.Now().Format(TIME_LAYOUT)
		a.Id = calcID([]byte(a.Title))
		a.URL = fmt.Sprintf("%s/%s", fatherURL, a.Id)
		return a, nil
	}

	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	if err := parseInto(a, bufio.NewReader(file)); err != nil {
		return nil, err
	}

	metaUpdated := false
	if a.Title == "" {
		metaUpdated = true
		a.Title = strings.TrimSuffix(pathpkg.Base(path), ".md")
	}
	if a.CreateTime == "" {
		metaUpdated = true
		a.CreateTime = time.Now().Format(TIME_LAYOUT)
	}
	if a.Id == "" {
		metaUpdated = true
		a.Id = calcID(a.Content)
	}
	if a.URL == "" {
		metaUpdated = true
		a.URL = fmt.Sprintf("%s/%s", fatherURL, a.Id)
	}

	if metaUpdated {
		if err := rewriteFrontMatter(file, a); err != nil {
			return a, err
		}
	}

	finalizeDerived(a)
	return a, nil
}

// parseInto reads YAML-ish front-matter + body into a, leaving derived fields
// (Tags, Summary, WordCount) for finalizeDerived.
func parseInto(a *Article, reader *bufio.Reader) error {
	stat := START
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			if len(line) > 0 && stat != META_BEGIN {
				a.Content = append(a.Content, line...)
			}
			break
		}
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		switch stat {
		case START:
			if strings.HasPrefix(string(line), "---") {
				stat = META_BEGIN
			} else {
				stat = NO_META
				a.Content = append(a.Content, line...)
			}
		case NO_META:
			a.Content = append(a.Content, line...)
		case META_BEGIN:
			if strings.HasPrefix(string(line), "---") {
				stat = META_END
				continue
			}
			slice := bytes.SplitN(line, []byte(":"), 2)
			if len(slice) < 2 {
				continue
			}
			key := strings.TrimSpace(string(slice[0]))
			value := strings.TrimSpace(string(slice[1]))
			// Obsidian writes tags as a YAML array (`tags: [a, b]`). Strip
			// the brackets so parseTags sees a clean comma list.
			if key == "tags" {
				value = strings.TrimPrefix(value, "[")
				value = strings.TrimSuffix(value, "]")
			}
			v := reflect.ValueOf(&(a.Meta)).Elem()
			for i := 0; i < v.NumField(); i++ {
				field := v.Type().Field(i)
				tagName := field.Tag.Get("meta")
				if tagName == "" {
					tagName = strings.ToLower(field.Name)
				}
				if tagName == key {
					v.FieldByName(field.Name).Set(reflect.ValueOf(value))
				}
			}
		case META_END:
			a.Content = append(a.Content, line...)
		}
	}
	return nil
}

func rewriteFrontMatter(file *os.File, a *Article) error {
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("seek: %w", err)
	}
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}
	w := bufio.NewWriter(file)
	out := []byte("---\n")
	v := reflect.ValueOf(&(a.Meta)).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		out = append(out, []byte(field.Tag.Get("meta")+": "+v.FieldByName(field.Name).String()+"\n")...)
	}
	out = append(out, []byte("---\n")...)
	out = append(out, a.Content...)
	if _, err := w.Write(out); err != nil {
		return err
	}
	return w.Flush()
}

func finalizeDerived(a *Article) {
	a.Tags = parseTags(a.TagsRaw)
	a.WordCount = countWords(a.Content)
	if a.WordCount > 0 {
		a.ReadingTimeMin = (a.WordCount + wordsPerMinute - 1) / wordsPerMinute
	}
	if a.Description != "" {
		a.Summary = a.Description
	} else {
		a.Summary = bodySummary(a.Content, 160)
	}
}

func calcID(data []byte) string {
	ieee := crc32.NewIEEE()
	ieee.Write(data)
	return strconv.FormatUint(uint64(ieee.Sum32()), 16)
}

// Slugify returns a kebab-case ASCII slug for s. If the result is empty (e.g.
// CJK-only input), Slugify returns the empty string and the caller should fall
// back to a hash.
func Slugify(s string) string {
	var b strings.Builder
	prevDash := true
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
			prevDash = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		default:
			// drop everything else (punctuation, CJK, ...)
		}
	}
	out := strings.Trim(b.String(), "-")
	return out
}

// SlugOrHash returns Slugify(s) if non-empty, else a stable hex hash.
func SlugOrHash(s string) string {
	if slug := Slugify(s); slug != "" {
		return slug
	}
	return calcID([]byte(s))
}

// CalcID exposes the body-content hash for callers that want a stable id.
func CalcID(data []byte) string { return calcID(data) }

func parseTags(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		t = strings.TrimPrefix(t, "#") // accept Obsidian #tag form too
		t = strings.Trim(t, `"' `)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func countWords(content []byte) int {
	count := 0
	inWord := false
	for _, r := range string(content) {
		if isCJK(r) {
			count++
			inWord = false
			continue
		}
		if unicode.IsSpace(r) {
			inWord = false
			continue
		}
		if !inWord {
			count++
			inWord = true
		}
	}
	return count
}

func isCJK(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF:
		return true
	case r >= 0x3400 && r <= 0x4DBF:
		return true
	case r >= 0x3040 && r <= 0x30FF:
		return true
	case r >= 0xAC00 && r <= 0xD7AF:
		return true
	}
	return false
}

func bodySummary(content []byte, maxRunes int) string {
	var b strings.Builder
	skipBlock := false
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			skipBlock = !skipBlock
			continue
		}
		if skipBlock || trimmed == "" {
			continue
		}
		trimmed = strings.TrimLeft(trimmed, "#>*-+ \t")
		if trimmed == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(trimmed)
		if utf8RuneLen(b.String()) >= maxRunes {
			break
		}
	}
	return truncateRunes(b.String(), maxRunes)
}

func utf8RuneLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i] + "…"
		}
		count++
	}
	return s
}
