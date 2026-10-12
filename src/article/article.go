package article

import (
	"bufio"
	"bytes"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
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
	Title       string `meta:"title"`
	Description string `meta:"description"`
	Author      string `meta:"author"`
	CreateTime  string `meta:"create_time"`
	UpdatedTime string `meta:"updated_time"`
	Category    string `meta:"category"`
	TagsRaw     string `meta:"tags"`
	Id          string `meta:"id"`
	URL         string `meta:"url"`
	Draft       string `meta:"draft"`
	Publish     string `meta:"publish"`
	// Pin sticks an article to the top of its containing listing,
	// overriding the usual create_time descending order.
	Pin string `meta:"pin"`
	// Private articles still appear in indexes but their body is gated
	// by HTTP Basic auth (see config [auth]). Lists show title + URL,
	// no Summary leak.
	Private string `meta:"private"`
	// Hidden articles are excluded from public routes and indexes.
	// Local preview can opt in through [blog].include_hidden.
	Hidden string `meta:"hidden"`
	// AI marks an AI-generated / -assisted post. Truthy values (true / 1 /
	// yes / on) render a generic "AI" badge; any other non-empty string
	// is treated as the model name and shown verbatim ("claude", "gpt-4o").
	AI string `meta:"ai"`
	// Cover is an optional hero image — relative path resolved against the
	// blog's image index, or an absolute URL. Renders at the top of the
	// post page and is also used for OG meta.
	Cover string `meta:"cover"`
	// Order, when present, makes the article participate in a sequential
	// (series-style) ordering inside its parent group. Integer-as-string.
	// See SeriesOrderKey + IsSequentialSeries in blog.go.
	Order         string `meta:"order"`
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

	frontMatter *yaml.Node
	parsedHTML  atomic.Value
}

func (a *Article) CachedHTML() (string, bool) {
	if v := a.parsedHTML.Load(); v != nil {
		return v.(string), true
	}
	return "", false
}

func (a *Article) StoreHTML(html string) { a.parsedHTML.Store(html) }

// IsDraft reports whether front-matter marks this article as a draft.
func (a *Article) IsDraft() bool {
	// Missing publish preserves historical behavior; explicit non-truthy
	// values withhold publication. Draft always takes precedence.
	return truthy(a.Draft) || (strings.TrimSpace(a.Publish) != "" && !truthy(a.Publish))
}

// IsPinned sticks the article to the top of its containing listing,
// overriding create_time order.
func (a *Article) IsPinned() bool { return truthy(a.Pin) }

// IsPrivate gates the article body behind HTTP Basic auth.
func (a *Article) IsPrivate() bool { return truthy(a.Private) }

// IsHidden excludes the article from public routes and indexes.
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

// AILabel returns the human-facing badge text. The intent borrows IPTC's
// three-tier digitalSourceType vocabulary (trained / composite / enhanced)
// in plain Chinese:
//
//	front-matter `ai:` value         badge        meaning
//	──────────────────────────────   ──────────   ─────────────────────────────
//	true / yes / 1 / on / assisted   AI 辅助       human-led, AI helped along
//	generated / wrote / written      AI 生成       AI-led, human reviewed/edited
//	edited / reviewed / polished     AI 校对       human-written, AI polished
//	<anything else non-empty>        <verbatim>   model byline ("claude", "gpt-4o", …)
//	false / no / 0 / off / empty     (no badge)
//
// Defaulting truthy values to "AI 辅助" matches the most idiomatic Chinese
// tech-blog convention (sspai et al.) and keeps the badge to two
// characters so it fits naturally in the post-meta strip.
func (a *Article) AILabel() string {
	if !a.IsAI() {
		return ""
	}
	v := strings.TrimSpace(a.AI)
	switch strings.ToLower(v) {
	case "true", "yes", "1", "on", "assisted", "assist", "ai-assisted":
		return "AI 辅助"
	case "generated", "ai-generated", "wrote", "written":
		return "AI 生成"
	case "edited", "ai-edited", "reviewed", "polished", "proofread":
		return "AI 校对"
	}
	return v
}

func (a *Article) IsGroup() bool { return len(a.SubArticle) > 0 }

// SeriesOrderKey returns the article's position in a sequential series.
// Resolution order:
//
//  1. front-matter `order: <n>` — explicit and wins.
//  2. leading digits in the file basename — `00-导读.md` → 0, `12-foo.md`
//     → 12. Handles the common Obsidian convention of prefixing series
//     chapters with two-digit numbers.
//  3. -1, meaning "no series signal" — the article should sort by date,
//     not by sequence.
//
// Used by blog.loadGroupDir to decide whether a group is a series (all
// leaves return ≥ 0) and how to order it.
func (a *Article) SeriesOrderKey() int {
	if v := strings.TrimSpace(a.Order); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	if a.Source != "" {
		base := a.Source
		if i := strings.LastIndexAny(base, "/\\"); i >= 0 {
			base = base[i+1:]
		}
		base = strings.TrimSuffix(base, ".md")
		n, ok := leadingDigits(base)
		if ok {
			return n
		}
	}
	return -1
}

// leadingDigits parses 0+ leading ASCII digits from s. Returns the parsed
// integer and true when at least one digit was found; (0, false) otherwise.
func leadingDigits(s string) (int, bool) {
	n := 0
	any := false
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
		any = true
		if n > 1<<20 {
			break // pathologically long prefix; cap so we don't overflow
		}
	}
	return n, any
}

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

// Less drives the default Articles sort: pinned first, then newest-first by
// CreateTime. Groups and leaf articles are mixed together — group dates
// already track their newest sub-article (set in blog.loadGroupDir), so a
// series that gets a new chapter floats back to the top naturally. The
// older "groups always above leaves" rule buried fresh standalone posts
// below every series; we don't do that any more.
//
// Series groups (`AI 入门到精通系列`-style) want ascending sequential
// order instead of date-desc. Those re-sort with a different comparator
// inside loadGroupDir — Less here only governs the default listings
// (home, tags, search).
func (a Articles) Less(i, j int) bool {
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
// missing meta — callers fill URL/Id/Title/CreateTime derivatively from the
// path (and may persist the result back via RewriteFrontMatter).
func ParseFile(path string) (*Article, error) {
	// Do not log source filenames: static builds can run in public CI.
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

// RewriteFrontMatter persists a.Meta back to a.Source. It opens the source
// file with O_RDWR, seeks to 0, truncates and writes a fresh `---`-delimited
// header followed by a.Content. Caller is responsible for ensuring a.Source
// is set and that a.Content is the body without front-matter (which is what
// ParseFile leaves you with).
//
// Used by the blog scanner to persist auto-generated id/url/title/create_time
// the first time it sees a note — the original gobog behavior, retained as
// product design.
func RewriteFrontMatter(a *Article) error {
	if a.Source == "" {
		return fmt.Errorf("rewrite: article has no source path")
	}
	file, err := os.OpenFile(a.Source, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", a.Source, err)
	}
	defer file.Close()
	return rewriteFrontMatter(file, a)
}

// parseInto reads YAML-ish front-matter + body into a, leaving derived fields
// (Tags, Summary, WordCount) for finalizeDerived.
func parseInto(a *Article, reader *bufio.Reader) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	lines := bytes.SplitAfter(data, []byte("\n"))
	if len(lines) == 0 || strings.TrimSpace(string(lines[0])) != "---" {
		a.Content = data
		return nil
	}
	var header []byte
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(string(lines[i])) == "---" {
			var doc yaml.Node
			if err := yaml.Unmarshal(header, &doc); err != nil {
				return fmt.Errorf("front matter in %s: %w", a.Source, err)
			}
			if len(doc.Content) > 0 {
				mapping := doc.Content[0]
				if mapping.Kind != yaml.MappingNode {
					return fmt.Errorf("front matter in %s must be a YAML mapping", a.Source)
				}
				var validated map[string]interface{}
				if err := mapping.Decode(&validated); err != nil {
					return fmt.Errorf("front matter in %s: %w", a.Source, err)
				}
				a.frontMatter = mapping
				meta := reflect.ValueOf(&a.Meta).Elem()
				for j := 0; j < len(mapping.Content); j += 2 {
					key, value := mapping.Content[j].Value, mapping.Content[j+1]
					for k := 0; k < meta.NumField(); k++ {
						if meta.Type().Field(k).Tag.Get("meta") != key {
							continue
						}
						text := ""
						if value.Tag == "!!null" {
							text = ""
						} else if key == "tags" && value.Kind == yaml.SequenceNode {
							var tags []string
							for _, tag := range value.Content {
								if tag.Kind != yaml.ScalarNode {
									return fmt.Errorf("tags in %s must be scalars", a.Source)
								}
								tags = append(tags, tag.Value)
							}
							text = strings.Join(tags, ",")
						} else if value.Kind == yaml.ScalarNode {
							text = value.Value
						} else {
							return fmt.Errorf("%s in %s must be a scalar", key, a.Source)
						}
						meta.Field(k).SetString(text)
					}
				}
			}
			a.Content = bytes.Join(lines[i+1:], nil)
			return nil
		}
		header = append(header, lines[i]...)
	}
	return fmt.Errorf("front matter in %s has no closing ---", a.Source)
}

func rewriteFrontMatter(file *os.File, a *Article) error {
	// Preserve unknown properties, YAML lists and comments when adding defaults.
	mapping := a.frontMatter
	if mapping == nil {
		mapping = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	meta := reflect.ValueOf(&a.Meta).Elem()
	for i := 0; i < meta.NumField(); i++ {
		key, value := meta.Type().Field(i).Tag.Get("meta"), meta.Field(i).String()
		found := -1
		for j := 0; j < len(mapping.Content); j += 2 {
			if mapping.Content[j].Value == key {
				found = j + 1
				break
			}
		}
		if found >= 0 {
			old := mapping.Content[found]
			// A sequence of tags is already represented in TagsRaw; keep its node.
			if key == "tags" && old.Kind == yaml.SequenceNode {
				continue
			}
			if old.Value == value || (old.Tag == "!!null" && value == "") {
				continue
			}
			mapping.Content[found] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
		} else if value != "" {
			mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
		}
	}
	var out bytes.Buffer
	out.WriteString("---\n")
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(mapping); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	out.WriteString("---\n")
	out.Write(a.Content)
	// Serialize before truncating, so invalid metadata never destroys a note.
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("seek: %w", err)
	}
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}
	_, err := file.Write(out.Bytes())
	return err
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
