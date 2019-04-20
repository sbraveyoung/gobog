package article

import (
	"strings"
	"time"

	"github.com/SmartBrave/gobog/pkg/user"
)

type ArticleType struct {
	Id          string
	FilePath    string
	Url         string
	Title       string
	Description string
	Author      string
	Time        string //time of write this article
	ModifyTime  int64  //time of create article file. also publish
	Content     []byte
	Parse       string
	Comments    []comment
	Tag         int
	SubArticle  ArticlesType
}

type ArticlesType []*ArticleType

func (a ArticlesType) Len() int {
	return len(a)
}

func (a ArticlesType) Less(i, j int) bool {
	if strings.Contains(a[i].Title, "杨智勇") {
		return false
	}
	if strings.Contains(a[j].Title, "杨智勇") {
		return true
	}
	if strings.Contains(strings.ToLower(a[i].Title), "about") {
		return false
	}
	if strings.Contains(strings.ToLower(a[j].Title), "about") {
		return true
	}
	if a[i].SubArticle == nil && a[j].SubArticle != nil {
		return false
	}
	if a[i].SubArticle != nil && a[j].SubArticle == nil {
		return true
	}
	//return a[i].ModifyTime > a[j].ModifyTime
	ti, err := time.Parse("2006-01-02 15:04:05", a[i].Time)
	if err != nil {
		return false
	}
	tj, err := time.Parse("2006-01-02 15:04:05", a[j].Time)
	if err != nil {
		return true
	}
	return ti.Unix() > tj.Unix()
}

func (a ArticlesType) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a *ArticleType) IsSame(id string) bool {
	return strings.Compare(a.Id, id) == 0
}

type comment struct {
	Id               int
	ArticleId        int
	Publisher        user.User //must login
	ReponseCommentId int
	Time             string
}
