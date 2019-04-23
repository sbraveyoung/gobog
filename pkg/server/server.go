package server

//try to use channel and goroutine

import (
	"crypto/tls"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	ttemplate "text/template"

	"github.com/SmartBrave/gobog/pkg/article"
	"github.com/SmartBrave/gobog/pkg/config"
	"github.com/SmartBrave/gobog/pkg/dao"
	httpc "github.com/SmartBrave/gobog/pkg/httpc"
	"github.com/SmartBrave/gobog/pkg/search"
	"github.com/astaxie/beego/logs"

	//"github.com/SmartBrave/gobog/pkg/markdown"
	"github.com/facebookgo/grace/gracehttp"
	"github.com/russross/blackfriday"
	//"github.com/golang-commonmark/markdown"
)

var (
	engine search.Engine
)

type Blog struct {
	conf     *config.Config
	articles article.ArticlesType
}

func New(c *config.Config) Blog {
	return Blog{
		conf: c,
	}
}

func (blog *Blog) init() {
	var err error
	engine, err = search.New()
	if err != nil {
		logs.Error(err)
		os.Exit(1)
	}

	posts, err := os.Open(fmt.Sprintf("%s/%s", blog.conf.Blog.Source, config.DIRS["posts"]))
	if err != nil {
		logs.Error(err)
		os.Exit(1)
	}

	files, err := posts.Readdir(0)
	if err != nil {
		logs.Error(err)
		os.Exit(1)
	}

	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, ".") {
			logs.Warn(name, " is a hidly file.")
			continue
		}
		path := fmt.Sprintf("%s/%s/%s", blog.conf.Blog.Source, config.DIRS["posts"], name)
		art, err := article.NewArticle(path, blog.conf.Blog.Author)
		if err != nil {
			logs.Error(err)
			continue
		}

		if art.Tag == article.DIR {
			subPosts, err := os.Open(path)
			if err != nil {
				logs.Warn(err)
				continue
			}
			subFiles, err := subPosts.Readdir(0)
			if err != nil {
				logs.Warn(err)
				continue
			}

			for _, subFile := range subFiles {
				subName := subFile.Name()
				if strings.HasPrefix(subName, ".") {
					logs.Warn(subName, " is a hidly file.")
					continue
				}
				subPath := fmt.Sprintf("%s/%s", path, subName)
				subArt, err := article.NewArticle(subPath, blog.conf.Blog.Author, art.Id)
				if err != nil {
					logs.Error(err)
					continue
				}
				if subArt.Tag == article.DIR {
					continue
				}
				logs.Info("init ", subPath, " success!")
				art.SubArticle = append(art.SubArticle, &subArt)
			}
			sort.Sort(art.SubArticle)
		} else {
			logs.Info("init ", path, " success!")
		}
		blog.articles = append(blog.articles, &art)
	}
	sort.Sort(blog.articles)

	for index, art := range blog.articles {
		if art.Tag == article.FILE {
			engine.Cut(strconv.Itoa(index), string(art.Content))
		} else {
			for jndex, subArt := range art.SubArticle {
				engine.Cut(strconv.Itoa(jndex), string(subArt.Content))
			}
		}
	}
}

func (blog *Blog) Run() error {
	blog.init()
	servers := []*http.Server{}
	cer, err := tls.LoadX509KeyPair(blog.conf.Http.Cert, blog.conf.Http.Key)
	if err != nil {
		logs.Error("generate cert fail.err: ", err)
		return err
	}
	for index, a := range blog.conf.Http.Addr {
		addr := *flag.String("addr"+strconv.Itoa(index), ":"+a, "blog listen on this addr.")
		servers = append(servers, &http.Server{
			Addr: addr,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				host := r.Host
				ret := strings.IndexByte(host, ':')
				if ret < 0 {
					logs.Warn("ret < 0,ret:", ret)
					ret = len(host)
				}
				host = host[:ret] + ":" + blog.conf.Http.Addrs[index] //this require that len(c.Http.Addr) must equal to len(c.Http.Addrs),and every elements should correspond.
				http.Redirect(w, r, fmt.Sprintf("https://%s%s", host, r.URL), http.StatusMovedPermanently)
			}),
		})
	}
	for index, a := range blog.conf.Http.Addrs {
		addrs := *flag.String("addrs"+strconv.Itoa(index), ":"+a, "blog listen on this addr.")
		servers = append(servers, &http.Server{
			Addr:    addrs,
			Handler: blog.newHandler(),
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cer},
			},
		})
	}
	if err := gracehttp.Serve(servers...); err != nil {
		logs.Error(err)
		panic("gracehttp.Serve occur some error: " + err.Error())
	}
	return nil
}

func (blog *Blog) newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", logMiddle(blog.rootHandler))
	mux.HandleFunc("/posts/", logMiddle(blog.postHandler))
	mux.HandleFunc("/images/", logMiddle(blog.imageHandler))
	mux.HandleFunc("/css/", logMiddle(blog.cssHandler))
	mux.HandleFunc("/js/", logMiddle(blog.jsHandler))
	mux.HandleFunc("/videos/", logMiddle(blog.videoHandler))
	mux.HandleFunc("/audios/", logMiddle(blog.audioHandler))
	mux.HandleFunc("/about", logMiddle(blog.aboutHandler))
	mux.HandleFunc("/404", logMiddle(blog.notFoundHandler))
	mux.HandleFunc("/version", logMiddle(blog.versionHandler))
	mux.HandleFunc("/resume", logMiddle(blog.resumeHandler))
	mux.HandleFunc("/bing_img", logMiddle(blog.bingImgHandler))
	mux.HandleFunc("/search", logMiddle(blog.searchHandler))
	// mux.HandleFunc("/login", loogMiddle(loginHandler))

	return mux
}

func logMiddle(f func(http.ResponseWriter, *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.Info(fmt.Sprintf("%s %s %s %v %v", r.Method, r.URL, r.Host, r.Header, r.Body))
		f(w, r)
	}
}

func (blog *Blog) searchHandler(w http.ResponseWriter, r *http.Request) {
	indexs := []string{}
	switch r.Method {
	case "GET":
		query := r.FormValue("query")
		if query == "" {
			//when press register button
			w.Write([]byte("nil."))
			return
		} else {
			querys := strings.Split(query, "+")
			indexs = engine.Search(querys)
		}
	default:
		w.WriteHeader(http.StatusBadRequest)
	}

	articles := article.ArticlesType{}
	for _, index := range indexs {
		n, err := strconv.Atoi(index)
		if err != nil {
			//log
			continue
		}
		if n >= len(blog.articles)-2 {
			continue
		}
		articles = append(articles, blog.articles[n])
	}
	t, _ := template.ParseFiles(blog.conf.Blog.Theme + "/index.html")
	err := t.Execute(w, articles)
	if err != nil {
		logs.Error("t.Execute occur some err: ", err)
		//w.WriteHeader(http.StatusInternalServerError)
	}

}

func (blog *Blog) rootHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	if strings.Compare(url, "/") != 0 {
		logs.Error("not root url.")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(url + " is not found."))
		return
	}
	t, _ := template.ParseFiles(blog.conf.Blog.Theme + "/index.html")
	err := t.Execute(w, blog.articles[:len(blog.articles)-2])
	if err != nil {
		logs.Error("t.Execute occur some err: ", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (blog *Blog) postHandler(w http.ResponseWriter, r *http.Request) {
	//BUG:has error when url is '/posts/abc/dev' when '/posts/abc' is a article .
	url := r.URL.Path
	url = strings.TrimLeft(url, "/")
	paths := strings.Split(url, "/")
	if strings.Compare(paths[0], config.DIRS["posts"]) == 0 {
		paths = paths[1:]
		if len(paths) < 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	} else {
		logs.Warn(url)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var art article.ArticlesType
	pa := blog.articles[:len(blog.articles)-2]
	for len(paths) != 0 {
	label:
		for _, article := range pa {
			if article.IsSame(paths[0]) {
				if len(paths) == 1 {
					if len(article.SubArticle) > 0 {
						art = append(art, article.SubArticle...)
					} else {
						art = append(art, article)
					}
				} else {
					pa = article.SubArticle
				}
				break label
			}
		}
		paths = paths[1:]
	}
	if len(art) == 0 {
		logs.Warn("not found")
		w.WriteHeader(http.StatusNotFound)
		return
	} else if len(art) == 1 {
		//tmp := blackfriday.MarkdownBasic(append([]byte("## "+a[0].Title+"\n"), a[0].Content...))
		tmp := blackfriday.Markdown(append([]byte("## "+art[0].Title+"\n"), art[0].Content...), blackfriday.HtmlRenderer(0|blackfriday.HTML_USE_XHTML, "", ""), blackfriday.EXTENSION_FENCED_CODE|blackfriday.EXTENSION_TABLES)

		art[0].Parse = string(tmp) //TODO: should Parse article only access it first .
		t, err := ttemplate.ParseFiles(blog.conf.Blog.Theme + "/post.html")
		if err != nil {
			logs.Warn("t.ParseFiles occur some err: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		err = t.Execute(w, art[0])
		if err != nil {
			logs.Warn("t.Execute occur some err: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		t, err := ttemplate.ParseFiles(blog.conf.Blog.Theme + "/index.html")
		if err != nil {
			logs.Warn("t.ParseFiles occur some err: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		err = t.Execute(w, art)
		if err != nil {
			logs.Warn("t.Execute occur some err: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func (blog *Blog) imageHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	path := strings.TrimPrefix(url, fmt.Sprintf("/%s/", config.DIRS["images"]))
	args := strings.Split(path, "/")
	if len(args) < 1 {
		logs.Error(args)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	name := args[0]
	http.ServeFile(w, r, fmt.Sprintf("%s/%s/%s", blog.conf.Blog.Source, config.DIRS["images"], name)) //TODO:should support multiDir
}

func (blog *Blog) videoHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	path := strings.TrimPrefix(url, fmt.Sprintf("/%s/", config.DIRS["videos"]))
	args := strings.Split(path, "/")
	if len(args) < 1 {
		logs.Error(args)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	name := args[0]
	http.ServeFile(w, r, fmt.Sprintf("%s/%s/%s", blog.conf.Blog.Source, config.DIRS["videos"], name)) //TODO:should support multiDir
}
func (blog *Blog) audioHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	path := strings.TrimPrefix(url, fmt.Sprintf("/%s/", config.DIRS["audios"]))
	args := strings.Split(path, "/")
	if len(args) < 1 {
		logs.Error(args)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	name := args[0]
	http.ServeFile(w, r, fmt.Sprintf("%s/%s/%s", blog.conf.Blog.Source, config.DIRS["audios"], name)) //TODO:should support multiDir
}

func (blog *Blog) aboutHandler(w http.ResponseWriter, r *http.Request) {
	a := blog.articles[len(blog.articles)-2]
	tmp := blackfriday.Markdown(append([]byte("## "+a.Title+"\n"), a.Content...), blackfriday.HtmlRenderer(0|blackfriday.HTML_USE_XHTML, "", ""), blackfriday.EXTENSION_FENCED_CODE)

	a.Parse = string(tmp) //TODO: should Parse article only access it first .
	t, err := ttemplate.ParseFiles(blog.conf.Blog.Theme + "/post.html")
	if err != nil {
		logs.Warn("t.ParseFiles occur some err: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	err = t.Execute(w, a)
	if err != nil {
		logs.Warn("t.Execute occur some err: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (blog *Blog) cssHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	path := strings.TrimPrefix(url, "/css/")
	args := strings.Split(path, "/")
	if len(args) < 1 {
		logs.Error(args)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	name := args[0]
	http.ServeFile(w, r, fmt.Sprintf("%s/css/%s", blog.conf.Blog.Theme, name)) //TODO:should support multiDir
}
func (blog *Blog) jsHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	path := strings.TrimPrefix(url, "/js/")
	args := strings.Split(path, "/")
	if len(args) < 1 {
		logs.Error(args)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	name := args[0]
	http.ServeFile(w, r, fmt.Sprintf("%s/js/%s", blog.conf.Blog.Theme, name)) //TODO:should support multiDir
}

func (blog *Blog) bingImgHandler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			logs.Error("panic")
		}
	}()
	url := "https://cn.bing.com/HPImageArchive.aspx?format=js&idx=0&n=1"

	client := httpc.NewHttpClient(url).M("GET")
	respCode, data, err := client.Do()
	if err != nil || respCode != 200 {
		logs.Error(url, err)
		w.Write([]byte("Server Error"))
		return
	}
	var j resp
	err = data.Unmarshal(&j)
	if err != nil {
		logs.Error(url, err)
		w.Write([]byte("Server Error"))
		return
	}

	_, image, err := httpc.NewHttpClient("https://cn.bing.com" + j.Images[0].Url).M("GET").Do()
	if err != nil {
		logs.Error(err)
		w.Write([]byte("Server Error"))
		return
	}
	w.Write(image.Data)
}

func (blog *Blog) loginHandler(w http.ResponseWriter, r *http.Request) {

	logs.Info(r.URL)
	logs.Info(r.Method)
	logs.Info(r.Host)
	logs.Info(r.Header)
	logs.Info(r.Body)

	url := r.URL.Path
	fmt.Println(url)
	//MUST TODO
	//salt crypto
	//HTTPS
	switch r.Method {
	case "GET":
		//show login or register page
		t, _ := template.ParseFiles(blog.conf.Blog.Theme + "/login.html")
		login := Login{Title: blog.conf.Blog.Title, Info: ""}
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
				t, _ := template.ParseFiles(blog.conf.Blog.Theme + "/login.html")
				login := Login{Title: blog.conf.Blog.Title, Info: err.Error()}
				t.Execute(w, login)
			} else {
				w.Write([]byte("success"))
			}
		}
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

func (blog *Blog) notFoundHandler(w http.ResponseWriter, r *http.Request) {
}
func (blog *Blog) versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("V 0.1"))
}
func (blog *Blog) resumeHandler(w http.ResponseWriter, r *http.Request) {
	//need passwd,and it's availiable in some time.
	a := blog.articles[len(blog.articles)-1]
	tmp := blackfriday.Markdown(append([]byte("## "+a.Title+"\n"), a.Content...), blackfriday.HtmlRenderer(0|blackfriday.HTML_USE_XHTML, "", ""), blackfriday.EXTENSION_FENCED_CODE)

	a.Parse = string(tmp) //TODO: should Parse article only access it first .
	t, err := ttemplate.ParseFiles(blog.conf.Blog.Theme + "/resume.html")
	if err != nil {
		//log
		fmt.Println("t.ParseFiles occur some err: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	err = t.Execute(w, a)
	if err != nil {
		//log
		fmt.Println("t.Execute occur some err: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
