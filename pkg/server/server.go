package server

import (
	"flag"
	"fmt"
	"hash/crc32"
	"html/template"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/SmartBrave/gobog/pkg/config"
	"github.com/SmartBrave/gobog/pkg/dao"
	"github.com/SmartBrave/gobog/pkg/id"
	"github.com/SmartBrave/gobog/pkg/markdown"
	"github.com/facebookgo/grace/gracehttp"
)

var (
	c *config.Config
)

//blog path design:
//  /
//	/login
//	/posts/SHA1(article)
//	/about
//	/404
func New(conf *config.Config) {
	c = conf
	addr := *flag.String("addr", ":"+c.Http.Addr, "blog listen on this addr.")
	gracehttp.Serve(&http.Server{Addr: addr, Handler: newHandler()})
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/posts/", postsHandler)
	mux.HandleFunc("/about", aboutHandler)
	mux.HandleFunc("/404", notFoundHandler)
	mux.HandleFunc("/test", testHandler)

	return mux
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	//读取方式需要改变一下，不能每次访问主页都读取文件夹且读取每个文件
	file, err := os.Open("_source/post")
	if err != nil {
		w.Write([]byte("this article not exists!"))
	}
	article_files, err := file.Readdir(0)
	if err != nil {
		//log
		w.Write([]byte("some error occur."))
		return
	}
	for _, article_file := range article_files {
		if !article_file.IsDir() {
			name := article_file.Name()
			ieee := crc32.NewIEEE()
			ieee.Write([]byte(name))
			s := strconv.FormatUint(uint64(ieee.Sum32()), 16)
			Articles = append(Articles, &Article{
				FileName: name,
				Url:      "/posts/" + s,
			})
		}
	}
	t, _ := template.ParseFiles(c.Blog.Theme + "/index.html")
	t.Execute(w, Articles)
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	buf := make([]byte, 10000)
	var md markdown.Markdown
	var out string
	file, err := os.Open("_source/post/test.md")
	if err != nil {
		w.Write([]byte("this article not exists!"))
	}
	for {
		n, err := file.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			w.Write([]byte("some error occur."))
			//log
			return
		}
		md.OriginialText += string(buf[:n])
	}
	out, err = md.Parse()
	if err != nil {
		//log
		w.Write([]byte("unmarshal markdown fail."))
		return
	}
	fmt.Println(md.OriginialText)
	fmt.Println(out)
	w.Write([]byte(out))
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
	//url := r.URL.Path
	//articleId := strings.TrimPrefix(url, "/posts/")
	//id := strconv.Atoi(articleId)
	//w.Write([]byte(articleId))
	var md markdown.Markdown
	md.Parse()
	w.Write([]byte(strconv.FormatInt(id.Generate(), 10) + "\n"))
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("This is about page!"))
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
}
