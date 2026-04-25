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

const (
	TIME_LAYOUT = "2006-01-02 15:04:05"
)

const wordsPerMinute = 250

//front-matter: https://jekyllrb.com/docs/front-matter/
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

// CachedHTML returns the lazily-rendered HTML body and whether it was set.
func (a *Article) CachedHTML() (string, bool) {
	if v := a.parsedHTML.Load(); v != nil {
		return v.(string), true
	}
	return "", false
}

// StoreHTML caches the rendered HTML body. Safe for concurrent callers.
func (a *Article) StoreHTML(html string) {
	a.parsedHTML.Store(html)
}

// IsDraft reports whether the front-matter marks this article as a draft.
func (a *Article) IsDraft() bool {
	d := strings.ToLower(strings.TrimSpace(a.Draft))
	return d == "true" || d == "1" || d == "yes"
}

// IsGroup reports whether this article is a directory grouping sub-articles.
func (a *Article) IsGroup() bool { return len(a.SubArticle) > 0 }

type Articles []*Article

func (a Articles) Len() int      { return len(a) }
func (a Articles) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a Articles) Less(i, j int) bool {
	if a[i].SubArticle == nil && a[j].SubArticle != nil {
		return false
	}
	if a[i].SubArticle != nil && a[j].SubArticle == nil {
		return true
	}
	ti, erri := time.Parse(TIME_LAYOUT, a[i].CreateTime)
	tj, errj := time.Parse(TIME_LAYOUT, a[j].CreateTime)
	if erri != nil || errj != nil {
		return false
	}
	return ti.Unix() > tj.Unix()
}

func NewArticle(path, articleType, fatherURL string) (*Article, error) {
	logs.Debug("in NewArticles,path:", path, " articleType:", articleType, " fatherURL:", fatherURL)
	article := &Article{Source: path}

	if articleType == DIR {
		article.Title = pathpkg.Base(path)
		article.CreateTime = time.Now().Format(TIME_LAYOUT)
		article.Id = calcID([]byte(article.Title))
		article.URL = fmt.Sprintf("%s/%s", fatherURL, article.Id)
		return article, nil
	}

	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		logs.Error("open error:", err, " path:", path)
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	stat := START
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			logs.Error("readbytes err:", err)
			return nil, err
		}

		switch stat {
		case START:
			//FIXME: can not use "---" for line in markdown, could use "***" instead.
			if strings.HasPrefix(string(line), "---") {
				stat = META_BEGIN
			} else {
				stat = NO_META
				article.Content = append(article.Content, line...)
			}
		case NO_META:
			article.Content = append(article.Content, line...)
		case META_BEGIN:
			if strings.HasPrefix(string(line), "---") {
				stat = META_END
			} else {
				slice := bytes.SplitN(line, []byte(":"), 2)
				if len(slice) < 2 {
					continue
				}
				key := strings.TrimSpace(string(slice[0]))
				value := strings.TrimSpace(string(slice[1]))

				v := reflect.ValueOf(&(article.Meta)).Elem()
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
			}
		case META_END:
			article.Content = append(article.Content, line...)
		default:
			//XXX
		}
	}

	metaUpdated := false
	if article.Title == "" {
		metaUpdated = true
		article.Title = strings.TrimSuffix(pathpkg.Base(path), ".md")
	}
	if article.CreateTime == "" {
		metaUpdated = true
		article.CreateTime = time.Now().Format(TIME_LAYOUT)
	}
	if article.Id == "" {
		metaUpdated = true
		article.Id = calcID(article.Content)
	}
	if article.URL == "" {
		metaUpdated = true
		article.URL = fmt.Sprintf("%s/%s", fatherURL, article.Id)
	}

	if metaUpdated {
		_, err = file.Seek(0, 0)
		if err != nil {
			logs.Error("seek err:", err)
			return nil, err
		}
		if err := file.Truncate(0); err != nil {
			logs.Error("truncate err:", err)
			return nil, err
		}

		writer := bufio.NewWriter(file)
		writeString := []byte("---\n")
		v := reflect.ValueOf(&(article.Meta)).Elem()
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			tagName := field.Tag.Get("meta")
			tagValue := v.FieldByName(field.Name).String()
			writeString = append(writeString, []byte(tagName+": "+tagValue+"\n")...)
		}
		writeString = append(writeString, []byte("---\n")...)
		writeString = append(writeString, article.Content...)
		if _, err := writer.Write(writeString); err != nil {
			logs.Error("writeString err:", err)
			return nil, err
		}
		if err := writer.Flush(); err != nil {
			logs.Error("flush err:", err)
			return article, err
		}
	}

	article.Tags = parseTags(article.TagsRaw)
	article.WordCount = countWords(article.Content)
	if article.WordCount > 0 {
		article.ReadingTimeMin = (article.WordCount + wordsPerMinute - 1) / wordsPerMinute
	}
	if article.Description != "" {
		article.Summary = article.Description
	} else {
		article.Summary = bodySummary(article.Content, 160)
	}
	return article, nil
}

func calcID(data []byte) string {
	ieee := crc32.NewIEEE()
	ieee.Write(data)
	return strconv.FormatUint(uint64(ieee.Sum32()), 16)
}

func parseTags(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// countWords counts ASCII whitespace-delimited words plus CJK runes.
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
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
		return true
	case r >= 0x3400 && r <= 0x4DBF: // CJK Extension A
		return true
	case r >= 0x3040 && r <= 0x30FF: // Hiragana / Katakana
		return true
	case r >= 0xAC00 && r <= 0xD7AF: // Hangul Syllables
		return true
	}
	return false
}

// bodySummary returns up to maxRunes runes of the body, stripping markdown
// punctuation that would look noisy in OG / feed descriptions.
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
		// Strip leading markdown markers.
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
