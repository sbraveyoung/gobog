package blog

import (
	"os"
	pathpkg "path"
	"sort"
	"strings"

	"github.com/SmartBrave/gobog/src/article"
	articlepkg "github.com/SmartBrave/gobog/src/article"
	"github.com/SmartBrave/gobog/src/config"
	"github.com/astaxie/beego/logs"
	"github.com/prometheus/common/log"
)

var (
	BlogTypes = map[string]string{
		"post": "post",
		// "draft":   "draft",
		// "offline": "offline",
		"about": "about",
	}
)

var (
	Blog *BlogST
)

type BlogST struct {
	Domain      string
	Name        string
	SubName     string
	Description string
	Author      string
	Theme       string
	Articles    map[string]article.Articles
}

func init() {
	Blog = &BlogST{
		Domain:      config.C.Blog.Domain,
		Name:        config.C.Blog.Title,
		SubName:     config.C.Blog.Subtitle,
		Description: config.C.Blog.Description,
		Author:      config.C.Blog.Author,
		Theme:       config.C.Blog.Theme,
		Articles:    make(map[string]article.Articles),
	}

	for _, tYpe := range BlogTypes {
		var articles articlepkg.Articles
		rootPath := pathpkg.Join(config.C.Blog.Source, tYpe)
		root, err := os.Open(rootPath)
		if err != nil {
			logs.Error("open source:", err)
			os.Exit(1)
		}
		names, err := root.Readdirnames(-1)
		if err != nil {
			logs.Error("read:", err)
			os.Exit(1)
		}
		root.Close()

		for _, name := range names {
			path := pathpkg.Join(rootPath, name)
			logs.Debug("name:", name, " path:", path)
			fileInfo, err := os.Lstat(path)
			if err != nil {
				logs.Error("os.Lstat err:", err)
				continue
			}

			if fileInfo.IsDir() {
				article, err := articlepkg.NewArticle(path, articlepkg.DIR, "/"+tYpe)
				if err != nil {
					logs.Error("NewArticle err:", err, " path:", path)
					continue
				}
				subRoot, err := os.Open(path)
				if err != nil {
					logs.Warn(err)
					continue
				}
				subNames, err := subRoot.Readdirnames(-1)
				if err != nil {
					logs.Warn(err)
					continue
				}
				subRoot.Close()

				for _, subName := range subNames {
					subPath := pathpkg.Join(path, subName)
					if !strings.HasSuffix(subPath, ".md") {
						log.Warn("this file is not markdown,path:", subPath)
						continue
					}
					subArticle, err := articlepkg.NewArticle(subPath, articlepkg.ARTICLE, article.URL)
					if err != nil {
						logs.Error("here:", err)
						continue
					}
					article.SubArticle = append(article.SubArticle, subArticle)
				}
				sort.Sort(article.SubArticle)
				articles = append(articles, article)
			} else {
				if !strings.HasSuffix(path, ".md") {
					log.Warn("this file is not markdown,path:", path)
					continue
				}
				article, err := articlepkg.NewArticle(path, articlepkg.ARTICLE, "/"+tYpe)
				if err != nil {
					logs.Error("newArticle err:", err)
					continue
				}
				articles = append(articles, article)
			}
		}
		sort.Sort(articles)
		Blog.Articles[tYpe] = articles
	}

}
