package article

import (
	"bufio"
	"bytes"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// NormalizeDefaults supplies missing identity fields. Existing permanent URLs,
// IDs and dates always win. Date defaults come from history or source mtime.
type NormalizeDefaults struct {
	Title, Author, ID, URL string
	Time                   time.Time
	UpdatedTime            time.Time
}

// NormalizeFrontMatter canonicalizes YAML properties without changing body bytes
// or effective draft/hidden/private flags. Unknown properties and comments survive.
func NormalizeFrontMatter(data []byte, defaults NormalizeDefaults) ([]byte, error) {
	a := &Article{}
	if err := parseInto(a, bufio.NewReader(bytes.NewReader(data))); err != nil {
		return nil, err
	}
	mapping := a.frontMatter
	if mapping == nil {
		mapping = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	fields := map[string]*yaml.Node{}
	keys := map[string]*yaml.Node{}
	for i := 0; i < len(mapping.Content); i += 2 {
		keys[mapping.Content[i].Value] = mapping.Content[i]
		fields[mapping.Content[i].Value] = mapping.Content[i+1]
	}
	set := func(key string, value interface{}) error {
		n := &yaml.Node{}
		if err := n.Encode(value); err != nil {
			return err
		}
		if old := fields[key]; old != nil {
			n.HeadComment, n.LineComment, n.FootComment = old.HeadComment, old.LineComment, old.FootComment
			n.Anchor = old.Anchor
		}
		fields[key] = n
		if keys[key] == nil {
			keys[key] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
		}
		return nil
	}
	fallback := func(value, def string) string {
		if value != "" {
			return value
		}
		return def
	}
	text := map[string]string{
		"title": fallback(a.Title, defaults.Title), "author": fallback(a.Author, defaults.Author),
		"id": fallback(a.Id, defaults.ID), "url": fallback(a.URL, defaults.URL),
		"create_time":  fallback(a.CreateTime, defaults.Time.Format(TIME_LAYOUT)),
		"updated_time": a.UpdatedTime,
		"description":  a.Description, "category": a.Category, "cover": a.Cover,
	}
	if text["id"] == "" || text["url"] == "" || text["title"] == "" {
		return nil, fmt.Errorf("normalization requires title, id and url or defaults")
	}
	if n := fields["updated_time"]; n == nil || n.Tag == "!!null" || n.Value == "" {
		text["updated_time"] = text["create_time"]
		if !defaults.UpdatedTime.IsZero() {
			text["updated_time"] = defaults.UpdatedTime.Format(TIME_LAYOUT)
		}
	}
	for k, v := range text {
		if err := set(k, v); err != nil {
			return nil, err
		}
	}
	tags := []string{}
	seen := map[string]bool{}
	for _, tag := range parseTags(a.TagsRaw) {
		if !seen[tag] {
			tags = append(tags, tag)
			seen[tag] = true
		}
	}
	if err := set("tags", tags); err != nil {
		return nil, err
	}
	for k, v := range map[string]bool{"draft": truthy(a.Draft), "publish": !a.IsDraft(), "hidden": a.IsHidden(), "private": a.IsPrivate(), "pin": a.IsPinned()} {
		if err := set(k, v); err != nil {
			return nil, err
		}
	}
	var ai interface{} = a.AI
	if !a.IsAI() {
		ai = false
	} else if truthy(a.AI) {
		ai = true
	}
	if err := set("ai", ai); err != nil {
		return nil, err
	}
	order := []string{"title", "author", "id", "url", "create_time", "updated_time", "description", "category", "tags", "draft", "publish", "hidden", "private", "pin", "ai", "cover"}
	content := []*yaml.Node{}
	used := map[string]bool{}
	for _, key := range order {
		if fields[key] != nil {
			content = append(content, keys[key], fields[key])
			used[key] = true
		}
	}
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i].Value
		if !used[key] {
			content = append(content, keys[key], fields[key])
			used[key] = true
		}
	}
	mapping.Content = content
	var out bytes.Buffer
	out.WriteString("---\n")
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(mapping); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	out.WriteString("---\n")
	out.Write(a.Content)
	return out.Bytes(), nil
}
