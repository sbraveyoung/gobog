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
	"time"

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

const (
//Category
)

//front-matter: https://jekyllrb.com/docs/front-matter/
type Meta struct {
	Title         string `meta:"title"`
	Description   string `meta:"description"`
	Author        string `meta:"author"`
	CreateTime    string `meta:"create_time"`
	Category      string `meta:"category"`
	Id            string `meta:"id"`
	URL           string `meta:"url"`
	TyporaRootURL string `meta:"typora-root-url"`
}

type Article struct {
	Meta
	Content    []byte
	Parse      string
	SubArticle Articles
}

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
	article := &Article{}

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
				slice := bytes.Split(line, []byte(":"))
				if len(slice) < 2 {
					//log.warn
				}
				key := strings.TrimSpace(string(slice[0]))
				value := strings.TrimSpace(string(bytes.Join(slice[1:], []byte{})))

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
		article.Title = pathpkg.Base(path)
		article.Title = strings.TrimRight(article.Title, ".md")
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

	_, err = file.Seek(0, 0)
	if err != nil {
		logs.Error("seek err:", err)
		return nil, err
	}

	if metaUpdated {
		writeString := []byte{}
		writer := bufio.NewWriter(file)
		writeString = append(writeString, []byte("---\n")...)
		v := reflect.ValueOf(&(article.Meta)).Elem()
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			tagName := field.Tag.Get("meta")
			tagValue := v.FieldByName(field.Name).String()
			writeString = append(writeString, []byte(tagName+": "+tagValue+"\n")...)
		}
		writeString = append(writeString, []byte("---\n")...)
		writeString = append(writeString, article.Content...)
		fmt.Println("writeString:", string(writeString))
		_, err = writer.WriteString(string(writeString))
		if err != nil {
			logs.Error("writeString err:", err)
			return nil, err
		}
		err = writer.Flush()
		if err != nil {
			logs.Error("flush err:", err)
			return article, err
		}
	}
	file.Close()
	return article, nil
}

func calcID(data []byte) string {
	ieee := crc32.NewIEEE()
	ieee.Write(data)
	return strconv.FormatUint(uint64(ieee.Sum32()), 16)
}
