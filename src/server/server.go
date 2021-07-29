package server

import (
	"crypto/tls"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path"
	"strings"
	ttemplate "text/template"

	articlepkg "github.com/SmartBrave/gobog/src/article"
	"github.com/SmartBrave/gobog/src/blog"
	"github.com/SmartBrave/gobog/src/config"
	httpc "github.com/SmartBrave/utils/easyhttpclient"
	"github.com/astaxie/beego/logs"
	"github.com/grace/gracehttp"
	"github.com/russross/blackfriday"
	//"github.com/SmartBrave/gobog/src/markdown"
	//"github.com/golang-commonmark/markdown"
)

type Server struct{}

var (
	server *Server
)

func init() {
	servers := []*http.Server{}
	certificate, err := tls.LoadX509KeyPair(config.C.Http.Cert, config.C.Http.Key)
	if err != nil {
		//XXX log
		fmt.Println("err:", err)
		os.Exit(1)
	}
	servers = append(servers, &http.Server{
		Addr:    config.C.Http.Addr,
		Handler: server.newHandler(),
		// Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// host := r.Host
		// ret := strings.IndexByte(host, ':')
		// if ret < 0 {
		// logs.Warn("ret < 0,ret:", ret)
		// ret = len(host)
		// }
		// host = host[:ret] + ":" + config.C.Http.Addrs
		// http.Redirect(w, r, fmt.Sprintf("https://%s%s", host, r.URL), http.StatusMovedPermanently)
		// }),
	})

	servers = append(servers, &http.Server{
		Addr:    config.C.Http.Addrs,
		Handler: server.newHandler(),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{certificate},
		},
	})
	if err := gracehttp.Serve(servers...); err != nil {
		logs.Error(err)
		panic("gracehttp.Serve occur some error: " + err.Error())
	}
}

func (s *Server) newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", logMiddle(rootHandler))
	mux.HandleFunc("/post/", logMiddle(postHandler))
	mux.HandleFunc("/about", logMiddle(aboutHandler))
	mux.HandleFunc("/image/", logMiddle(imageHandler))
	mux.HandleFunc("/css/", logMiddle(cssHandler))
	mux.HandleFunc("/js/", logMiddle(jsHandler))
	mux.HandleFunc("/bing_img", logMiddle(bingImgHandler))
	// mux.HandleFunc("/search", logMiddle(searchHandler))
	return mux
}

func logMiddle(f func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.Info(fmt.Sprintf("%s %s %s %v %v", r.Method, r.URL, r.Host, r.Header, r.Body))
		f(w, r)
		//XXX log response
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	if strings.Compare(url, "/") != 0 {
		w.WriteHeader(http.StatusNotFound)
		logs.Warn("error url:", url)
		return
	}
	t, _ := template.ParseFiles(config.C.Blog.Theme + "/index.html")
	//XXX 顶层 articles 可以看成一个 article，然后可以 set 顶层 article title 为 gobog，以便于在 html 模板中配置
	err := t.Execute(w, blog.Blog.Articles[blog.BlogTypes["post"]])
	if err != nil {
		logs.Error("t.Execute occur some err: ", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	var article *articlepkg.Article
	for _, p := range blog.Blog.Articles[blog.BlogTypes["post"]] {
		if strings.HasPrefix(r.URL.Path, p.URL) {
			article = p
			if r.URL.Path != p.URL && len(article.SubArticle) != 0 {
				for _, sp := range article.SubArticle {
					if r.URL.Path == sp.URL {
						article = sp
					}
				}
			}
			break
		}
	}
	if article == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if len(article.SubArticle) == 0 {
		//tmp := blackfriday.MarkdownBasic(append([]byte("## "+a[0].Title+"\n"), a[0].Content...))
		tmp := blackfriday.Markdown(append([]byte("## "+article.Title+"\n"), article.Content...), blackfriday.HtmlRenderer(0|blackfriday.HTML_USE_XHTML, "", ""), blackfriday.EXTENSION_FENCED_CODE|blackfriday.EXTENSION_TABLES)
		article.Parse = string(tmp)
		t, err := ttemplate.ParseFiles(config.C.Blog.Theme + "/post.html")
		if err != nil {
			logs.Warn("t.ParseFiles occur some err: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		err = t.Execute(w, article)
		if err != nil {
			logs.Warn("t.Execute occur some err: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		t, err := template.ParseFiles(config.C.Blog.Theme + "/index.html")
		if err != nil {
			logs.Warn("t.ParseFiles occur some err: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		err = t.Execute(w, article.SubArticle)
		if err != nil {
			logs.Warn("t.Execute occur some err: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	if len(blog.Blog.Articles[blog.BlogTypes["about"]]) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	article := blog.Blog.Articles[blog.BlogTypes["about"]][0]
	tmp := blackfriday.Markdown(append([]byte("## "+article.Title+"\n"), article.Content...), blackfriday.HtmlRenderer(0|blackfriday.HTML_USE_XHTML, "", ""), blackfriday.EXTENSION_FENCED_CODE)
	article.Parse = string(tmp)
	t, err := ttemplate.ParseFiles(config.C.Blog.Theme + "/post.html")
	if err != nil {
		logs.Warn("t.ParseFiles occur some err: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	err = t.Execute(w, article)
	if err != nil {
		logs.Warn("t.Execute occur some err: ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func imageHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, path.Join(config.C.Blog.Source, r.URL.Path))
}

func cssHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, path.Join(config.C.Blog.Theme, r.URL.Path))
}

func jsHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, path.Join(config.C.Blog.Theme, r.URL.Path))
}

func bingImgHandler(w http.ResponseWriter, r *http.Request) {
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

// func (blog *Blog) searchHandler(w http.ResponseWriter, r *http.Request) {
// indexs := []string{}
// switch r.Method {
// case "GET":
// query := r.FormValue("query")
// if query == "" {
// //when press register button
// w.Write([]byte("nil."))
// return
// } else {
// querys := strings.Split(query, "+")
// indexs = engine.Search(querys)
// }
// default:
// w.WriteHeader(http.StatusBadRequest)
// }

// articles := article.ArticlesType{}
// for _, index := range indexs {
// n, err := strconv.Atoi(index)
// if err != nil {
// //log
// continue
// }
// if n >= len(blog.articles)-2 {
// continue
// }
// articles = append(articles, blog.articles[n])
// }
// t, _ := template.ParseFiles(config.C.Blog.Theme + "/index.html")
// err := t.Execute(w, articles)
// if err != nil {
// logs.Error("t.Execute occur some err: ", err)
// //w.WriteHeader(http.StatusInternalServerError)
// }

// }
