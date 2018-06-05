package markdown

import (
	"fmt"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	arr := map[string]string{
		"# FirstTitle":         "<h1>FirstTitle</h1>",
		"### ThirdTitle":       "<h3>ThirdTitle</h3>",
		"###### SixthTitle":    "<h6>SixthTitle</h6>",
		"####### SeventhTitle": "<h6>SeventhTitle</h6>", //maybe is "#######SevenTitle"
		"## Has A Space Title": "<h2>Has&nbsp;A&nbsp;Space&nbsp;Title</h2>",
		"#### HasSharp#Title":  "<h4>HasSharp#Title</h4>",

		"*123*":   "<p><em>123</em></p>",
		"*## ##*": "<p><em>##&nbsp;##</em></p>",

		"***": "<hr>",

		"# TestTitleAnd\n*Italic*AndHr\n\n***": "<h1>TestTitleAnd</h1><p><em>Italic</em></p><p>AndHr</p><hr>",

		"* list1\n* list2\n* list3\n# Title": "<ul><li>list1</li><li>list2</li><li>list3</li></ul><h1>Title</h1>",
		"**strong**":                         "<p><strong>strong</strong></p>",

		"![image/description](image/path)": "<p><img alt=\"image/description\" src=\"image/path\"></p>",

		"[description](url)\n": "<p><a href=\"url\">description</a></p>",
		"> string":             "<blockquote><p>string</p></blockquote>",
		"> string\n\n## title": "<blockquote><p>string</p></blockquote><h2>title</h2>",
	}
	for key, value := range arr {
		var md Markdown
		md.OriginialText = []rune(key)
		out, err := md.Parse()
		if err != nil {
			fmt.Println("Parse(" + key + "): err: " + err.Error())
		}
		if strings.Compare(string(out), value) != 0 {
			t.Error("Parse(" + key + "): want: " + value + ", is: " + string(out))
		}
	}
}
