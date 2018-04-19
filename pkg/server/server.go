package server

import (
	"flag"
	"net/http"
	"strconv"
	"strings"

	"github.com/facebookgo/grace/gracehttp"
)

var (
	s *http.Server
)

//blog path design:
//  /
//	/login
//	/posts/SHA1(article)
//	/about
//	/404
func New(addr string) {
	addr = *flag.String("addr", ":"+addr, "blog listen on this addr.")
	gracehttp.Serve(&http.Server{Addr: addr, Handler: newHandler()})
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/posts/", postsHandler)
	mux.HandleFunc("/about", aboutHandler)

	return mux
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("This is root page!"))
}
func loginHandler(w http.ResponseWriter, r *http.Request) {
	register := r.FormValue("register")
	if register != "" {
		w.Write([]byte("you are registering now."))
	} else {
		w.Write([]byte("you are registered before."))
	}
}
func postsHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	articleId := strings.TrimPrefix(url, "/posts/")
	id := strconv.Atoi(articleId)
	w.Write([]byte(articleId))
	//w.Write([]byte(strconv.FormatInt(id.Generate(), 10) + "\n"))
}
func aboutHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("This is about page!"))
}
