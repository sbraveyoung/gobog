package article

import (
	"bufio"
	"bytes"
	"testing"
	"time"
)

func TestNormalizePreservesBodyPermalinkDateAndUnknownYAML(t *testing.T) {
	before := []byte("---\ntitle: Old\nid: stable\nurl: /post/permanent\ncreate_time: 2018-01-02 12:00:00\ndraft: \"yes\" # do not publish\ntags: 'Go, #Obsidian'\naliases: [One, Two]\ncustom: # keep me\n  value: 42\n---\n\n# Body\r\nverbatim  \n")
	opts := NormalizeDefaults{Title: "fallback", Author: "author", ID: "new", URL: "/new", Time: time.Date(2026, 10, 12, 0, 0, 0, 0, time.Local)}
	after, err := NormalizeFrontMatter(before, opts)
	if err != nil {
		t.Fatal(err)
	}
	var a Article
	if err := parseInto(&a, bufio.NewReader(bytes.NewReader(after))); err != nil {
		t.Fatal(err)
	}
	if a.Id != "stable" || a.URL != "/post/permanent" || a.CreateTime != "2018-01-02 12:00:00" || !a.IsDraft() {
		t.Fatal("identity/date/visibility changed")
	}
	if !bytes.Equal(a.Content, []byte("\n# Body\r\nverbatim  \n")) {
		t.Fatal("body bytes changed")
	}
	if !bytes.Contains(after, []byte("# keep me")) || !bytes.Contains(after, []byte("aliases:")) || !bytes.Contains(after, []byte("value: 42")) {
		t.Fatal("unknown metadata/comments lost")
	}
	again, err := NormalizeFrontMatter(after, opts)
	if err != nil || !bytes.Equal(after, again) {
		t.Fatalf("normalization not idempotent: %v", err)
	}
}

func TestNormalizePreservesEffectivePublicationForLegacyFlags(t *testing.T) {
	for _, flags := range []string{"", "draft: true", "publish: false", "hidden: yes", "private: on", "publish: true\ndraft: true", "draft: false\npublish: no"} {
		t.Run(flags, func(t *testing.T) {
			before := []byte("---\n" + flags + "\n---\nBody")
			var a, b Article
			if err := parseInto(&a, bufio.NewReader(bytes.NewReader(before))); err != nil {
				t.Fatal(err)
			}
			after, err := NormalizeFrontMatter(before, NormalizeDefaults{Title: "T", Author: "A", ID: "id", URL: "/post/id", Time: time.Now()})
			if err != nil {
				t.Fatal(err)
			}
			if err := parseInto(&b, bufio.NewReader(bytes.NewReader(after))); err != nil {
				t.Fatal(err)
			}
			if a.IsDraft() != b.IsDraft() || a.IsHidden() != b.IsHidden() || a.IsPrivate() != b.IsPrivate() {
				t.Fatalf("visibility changed: %s", after)
			}
			if b.Publish == "" || b.Draft == "" || b.Hidden == "" || b.Private == "" {
				t.Fatal("explicit booleans missing")
			}
		})
	}
}
