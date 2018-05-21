package server

//channel goroutine应该用一下

import (
	"bufio"
	"flag"
	"fmt"
	"hash/crc32"
	"html/template"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	ttemplate "text/template"
	"time"

	"github.com/SmartBrave/gobog/pkg/config"
	"github.com/SmartBrave/gobog/pkg/dao"
	//"github.com/SmartBrave/gobog/pkg/markdown"
	"github.com/facebookgo/grace/gracehttp"
	"github.com/russross/blackfriday"
	//"github.com/golang-commonmark/markdown"
)

var (
	c        *config.Config
	articles Articles
)

func Init() {
	file, err := os.Open(c.Blog.PostPath)
	if err != nil {
		//log
		os.Exit(1)
	}
	article_files, err := file.Readdir(0)
	if err != nil {
		//log
		os.Exit(1)
	}
	for _, article_file := range article_files {
		name := article_file.Name()
		path := c.Blog.PostPath + "/" + name
		article, err := newArticle(path)
		if err != nil {
			//log
			continue
		}

		if article.tag == DIR {
			//maybe dir is a zhuanlan
			subFile, err := os.Open(path)
			if err != nil {
				//log
				continue
			}
			sub_article_files, err := subFile.Readdir(0)
			if err != nil {
				//log
				continue
			}
			for _, sub_article_file := range sub_article_files {
				subName := sub_article_file.Name()
				subPath := path + "/" + subName
				subArticle, err := newArticle(subPath)
				if err != nil {
					continue
				}
				if subArticle.tag == DIR {
					continue
				}
				article.SubArticle = append(article.SubArticle, subArticle)
			}
			sort.Sort(article.SubArticle)
		}
		articles = append(articles, article)
	}
	sort.Sort(articles)
}

//blog path design:
//  /
//	/login
//	/posts/SHA1(article)
//  /image
//	/about
//	/404
func New(conf *config.Config) {
	c = conf
	Init()
	addr := *flag.String("addr", ":"+c.Http.Addr, "blog listen on this addr.")
	gracehttp.Serve(&http.Server{Addr: addr, Handler: newHandler()})
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/posts/", postsHandler)
	mux.HandleFunc("/image/", imagesHandler)
	mux.HandleFunc("/about", aboutHandler)
	mux.HandleFunc("/404", notFoundHandler)
	//mux.HandleFunc("/test", testHandler)

	return mux
}

func imagesHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	path := strings.TrimPrefix(url, "/image/")
	args := strings.Split(path, "/")
	if len(args) < 1 {
		//log
		//w.WriteHeader(http.StatusBadRequest)
		//BUG: has no effect
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := args[0]
	http.ServeFile(w, r, c.Blog.ImagePath+"/"+name) //BUG:should support multiDir
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	t, _ := template.ParseFiles(c.Blog.Theme + "/index.html")
	var err error
	if strings.HasPrefix(url, "/zhuanlan/") {
		title := strings.TrimPrefix(url, "/zhuanlan/")
		for _, article := range articles {
			if article.Title == title && article.tag == DIR {
				err = t.Execute(w, article.SubArticle)
			}
		}
	} else {
		err = t.Execute(w, articles)
	}
	if err != nil {
		//log
		fmt.Println("t.Execute occur some err: ", err)
		//w.WriteHeader(http.StatusInternalServerError)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	//MUST TODO
	//salt crypto
	//HTTPS
	switch r.Method {
	case "GET":
		//show login or register page
		t, _ := template.ParseFiles(c.Blog.Theme + "/login.html")
		login := Login{Title: c.Blog.Title, Info: ""}
		t.Execute(w, login)
	case "POST":
		register := r.FormValue("register")
		if register != "" {
			//when press register button
			w.Write([]byte("you are registering now."))
		} else {
			//when press login button
			user := r.FormValue("user")
			passwd := r.FormValue("password")
			if err := dao.VerifyLogin(user, passwd); err != nil {
				t, _ := template.ParseFiles(c.Blog.Theme + "/login.html")
				login := Login{Title: c.Blog.Title, Info: err.Error()}
				t.Execute(w, login)
			} else {
				w.Write([]byte("success"))
			}
		}
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

func postsHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	path := strings.TrimPrefix(url, "/posts/")
	args := strings.Split(path, "/")
	if len(args) < 1 {
		//log
		//w.WriteHeader(http.StatusBadRequest)
		//BUG: has no effect
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := args[0]
	//var md markdown.Markdown
	for _, article := range articles {
		if article.IsSame(id) {
			//md.OriginialText = article.Content
			//tmp, err := md.Parse()

			tmp := blackfriday.MarkdownBasic(article.Content)

			//md := markdown.New(markdown.XHTMLOutput(true), markdown.Nofollow(true))
			//article.Parse = md.RenderToString(article.Content)

			//if err != nil {
			//	//log
			//	fmt.Println("md.Parse occur some err: ", err)
			//	w.WriteHeader(http.StatusInternalServerError)
			//	return
			//}
			article.Parse = string(tmp)
			t, err := ttemplate.ParseFiles(c.Blog.Theme + "/posts.html")
			if err != nil {
				//log
				fmt.Println("t.ParseFiles occur some err: ", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			/*
				t = t.Funcs(ttemplate.FuncMap{
					"ConvertRunetoString": func(r []rune) string {
						return string(r)
					}})
			*/
			err = t.Execute(w, article)
			if err != nil {
				//log
				fmt.Println("t.Execute occur some err: ", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			break
		}
	}
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
}

//filepath should is a absPath
func newArticle(filePath string) (*Article, error) {
	article := Article{
		FilePath: filePath,
		Author:   c.Blog.Author,
		tag:      FILE,
	}
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		//log
		return nil, err
	}

	sysInfo := fileInfo.Sys()
	if stat, ok := sysInfo.(*syscall.Stat_t); ok {
		mTime := time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec) //Mtime,because we can't get create time
		article.ModifyTime = mTime.Unix()
	}

	if fileInfo.IsDir() {
		article.tag = DIR
		article.Title = fileInfo.Name()
		article.Url = "/zhuanlan/" + article.Title
		return &article, nil
	}

	file, err := os.OpenFile(filePath, os.O_RDWR, 0666)
	if err != nil {
		//log
		//os.Exit(1) //should not exit
		return nil, err
	}
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			//log error
			break
		}
		if err != nil {
			//log
			return nil, err
		}
		article.Content = append(article.Content, []byte(line)...)
		if strings.HasPrefix(line, "---") {
			article.Content = []byte{}
			for {
				l, err := reader.ReadString('\n')
				if err == io.EOF {
					goto out
				}
				if err != nil {
					//log
					goto out
				}
				article.Content = append(article.Content, []byte(l)...)
			}
		}
		slice := strings.Split(line, ": ")
		if len(slice) != 2 {
			//log
			break
		}
		slice[1] = strings.TrimRight(slice[1], "\n")
		switch slice[0] {
		case "title", "Title", "TITLE":
			article.Title = slice[1]
		case "date", "Date", "DATE":
			article.Time = slice[1]
		case "author", "Author", "AUTHOR":
			article.Author = slice[1]
		case "url", "Url", "URL":
			article.Url = slice[1]
		case "description", "Description", "DESCRIPTION":
			article.Description = slice[1]
		case "id", "Id", "ID":
			article.Id = slice[1]
		default:
			//log
			continue
		}
	}
out:
	writeString := []byte{}
	if strings.Compare(article.Title, "") == 0 {
		article.Title = fileInfo.Name()
	}
	if strings.Compare(article.Id, "") == 0 {
		ieee := crc32.NewIEEE()
		ieee.Write([]byte(article.Title))
		s := strconv.FormatUint(uint64(ieee.Sum32()), 16)
		article.Id = s
	}
	if strings.Compare(article.Url, "") == 0 {
		article.Url = "/posts/" + article.Id
	}
	if strings.Compare(article.Time, "") == 0 {
		article.Time = time.Now().Format("2006-01-02 15:04:05") //the time of write this article is now default if have no date tag.
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		//log
		//goto out
	}
	//Future: could not write every time if this article is not publish first.
	writer := bufio.NewWriter(file)
	writeString = append(writeString, []byte("title: "+article.Title+"\n")...)
	writeString = append(writeString, []byte("date: "+article.Time+"\n")...)
	writeString = append(writeString, []byte("id: "+article.Id+"\n")...)
	writeString = append(writeString, []byte("url: "+article.Url+"\n")...)
	writeString = append(writeString, []byte("---\n")...)
	writeString = append(writeString, article.Content...)
	_, err = writer.WriteString(string(writeString))
	if err != nil {
		//log
		return nil, err
	}
	err = writer.Flush()
	if err != nil {
		//log
		fmt.Println(err)
	}
	return &article, nil
}
