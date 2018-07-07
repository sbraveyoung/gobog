package server

//channel goroutine应该用一下

import (
	"bufio"
	"crypto/tls"
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
	log "qiniupkg.com/x/log.v7"
	//"github.com/SmartBrave/gobog/pkg/log"
	//"github.com/SmartBrave/gobog/pkg/markdown"
	"github.com/facebookgo/grace/gracehttp"
	"github.com/russross/blackfriday"
	//"github.com/golang-commonmark/markdown"
)

var (
	c *config.Config
)

func Init() {
	//file, err := os.OpenFile(c.Log.Path, os.O_APPEND|os.O_CREATE, 0666)
	//if err != nil {
	//	fmt.Println("Open log file fail. path: ", c.Log.Path, " err: ", err)
	//	os.Exit(1) //should not exit
	//}
	//log.Init(file)
	//log.Info("print some infomation.")
	file, err := os.Open(c.Blog.PostPath)
	if err != nil {
		log.Debug("Open blog dir fail: " + c.Blog.PostPath)
		os.Exit(1)
	}
	article_files, err := file.Readdir(0)
	if err != nil {
		//log
		os.Exit(1)
	}
	for _, article_file := range article_files {
		name := article_file.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := c.Blog.PostPath + "/" + name
		article, err := newArticle(path, "")
		if err != nil {
			//log
			continue
		}

		if article.Tag == config.DIR {
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
				if strings.HasPrefix(subName, ".") {
					continue
				}
				subPath := path + "/" + subName
				subArticle, err := newArticle(subPath, article.Id)
				if err != nil {
					continue
				}
				if subArticle.Tag == config.DIR {
					continue
				}
				subArticle.Url = "/post/" + article.Id + "/" + subArticle.Id
				article.SubArticle = append(article.SubArticle, subArticle)
			}
			sort.Sort(article.SubArticle)
		}
		c.Blog.Articles = append(c.Blog.Articles, article)
	}
	sort.Sort(c.Blog.Articles)
}

func New(conf *config.Config) {
	c = conf
	Init()
	servers := []*http.Server{}
	cer, err := tls.LoadX509KeyPair(c.Http.Cert, c.Http.Key)
	if err != nil {
		//log
		fmt.Println("generate cert fail.err: ", err)
		return
	}
	for index, a := range c.Http.Addr {
		addr := *flag.String("addr"+strconv.Itoa(index), ":"+a, "blog listen on this addr.")
		servers = append(servers, &http.Server{
			Addr: addr,
			// FIXME: what's the http.HandlerFunc? why it can accept one arg?
			// Answer: convert anonymous function to type of http.HandlerFunc forced.
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				host := r.Host
				ret := strings.IndexByte(host, ':')
				if ret < 0 {
					//log
					ret = len(host)
				}
				host = host[:ret] + ":" + c.Http.Addrs[index] //this require that len(c.Http.Addr) must equal to len(c.Http.Addrs),and every elements should correspond.
				http.Redirect(w, r, fmt.Sprintf("https://%s%s", host, r.URL), http.StatusMovedPermanently)
			}),
		})
	}
	for index, a := range c.Http.Addrs {
		addrs := *flag.String("addrs"+strconv.Itoa(index), ":"+a, "blog listen on this addr.")
		servers = append(servers, &http.Server{
			Addr:    addrs,
			Handler: newHandler(),
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cer},
			},
		})
	}
	if err := gracehttp.Serve(servers...); err != nil {
		//log
		panic("gracehttp.Serve occur some error: " + err.Error())
	}
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/login", loginHandler)
	mux.HandleFunc("/post/", postHandler)
	mux.HandleFunc("/image/", imageHandler)
	mux.HandleFunc("/css/", cssHandler)
	mux.HandleFunc("/js/", jsHandler)
	mux.HandleFunc("/video/", videoHandler)
	mux.HandleFunc("/audio/", audioHandler)
	mux.HandleFunc("/about", aboutHandler)
	mux.HandleFunc("/resume", resumeHandler)
	//mux.HandleFunc("/test", testHandler)
	//mux.HandleFunc("/debug", debugHandler)

	return mux
}

func resumeHandler(w http.ResponseWriter, r *http.Request) {
	//need passwd,and it's availiable in some time.
	url := r.URL.Path
	fmt.Println(url)
	a := c.Blog.Articles[len(c.Blog.Articles)-1]
	tmp := blackfriday.Markdown(append([]byte("## "+a.Title+"\n"), a.Content...), blackfriday.HtmlRenderer(0|blackfriday.HTML_USE_XHTML, "", ""), blackfriday.EXTENSION_FENCED_CODE)

	a.Parse = string(tmp) //TODO: should Parse article only access it first .
	t, err := ttemplate.ParseFiles("themes/" + c.Blog.Theme + "/resume.html")
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

func videoHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	fmt.Println(url)
	path := strings.TrimPrefix(url, "/video/")
	args := strings.Split(path, "/")
	if len(args) < 1 {
		//log
		//w.WriteHeader(http.StatusBadRequest)
		//BUG: has no effect
		//FIXME
		//TODO
		//XXX
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := args[0]
	http.ServeFile(w, r, c.Blog.VideoPath+"/"+name) //TODO:should support multiDir
}
func audioHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	fmt.Println(url)
	path := strings.TrimPrefix(url, "/audio/")
	args := strings.Split(path, "/")
	if len(args) < 1 {
		//log
		//w.WriteHeader(http.StatusBadRequest)
		//BUG: has no effect
		//FIXME
		//TODO
		//XXX
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := args[0]
	http.ServeFile(w, r, c.Blog.AudioPath+"/"+name) //TODO:should support multiDir
}
func jsHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	fmt.Println(url)
	path := strings.TrimPrefix(url, "/js/")
	args := strings.Split(path, "/")
	if len(args) < 1 {
		//log
		//w.WriteHeader(http.StatusBadRequest)
		//BUG: has no effect
		//FIXME
		//TODO
		//XXX
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := args[0]
	http.ServeFile(w, r, c.Blog.JsPath+"/"+name) //TODO:should support multiDir
}
func cssHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	fmt.Println(url)
	path := strings.TrimPrefix(url, "/css/")
	args := strings.Split(path, "/")
	if len(args) < 1 {
		//log
		//w.WriteHeader(http.StatusBadRequest)
		//BUG: has no effect
		//FIXME
		//TODO
		//XXX
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := args[0]
	http.ServeFile(w, r, c.Blog.CssPath+"/"+name) //TODO:should support multiDir
}
func imageHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	fmt.Println(url)
	path := strings.TrimPrefix(url, "/image/")
	args := strings.Split(path, "/")
	if len(args) < 1 {
		//log
		//w.WriteHeader(http.StatusBadRequest)
		//BUG: has no effect
		//FIXME
		//TODO
		//XXX
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name := args[0]
	http.ServeFile(w, r, c.Blog.ImagePath+"/"+name) //TODO:should support multiDir
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	fmt.Println(url)
	if strings.Compare(url, "/") != 0 {
		//log
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(url + " is not found."))
		return
	}
	t, _ := template.ParseFiles("themes/" + c.Blog.Theme + "/index.html")
	err := t.Execute(w, c.Blog.Articles[:len(c.Blog.Articles)-2])
	if err != nil {
		//log
		fmt.Println("t.Execute occur some err: ", err)
		//w.WriteHeader(http.StatusInternalServerError)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	fmt.Println(url)
	//MUST TODO
	//salt crypto
	//HTTPS
	switch r.Method {
	case "GET":
		//show login or register page
		t, _ := template.ParseFiles("themes/" + c.Blog.Theme + "/login.html")
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
				t, _ := template.ParseFiles("themes/" + c.Blog.Theme + "/login.html")
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

func postHandler(w http.ResponseWriter, r *http.Request) {
	//BUG:has error when url is '/post/abc/dev' when '/post/abc' is a article .
	url := r.URL.Path
	fmt.Println(url)
	url = strings.TrimLeft(url, "/")
	paths := strings.Split(url, "/")
	if strings.Compare(paths[0], "post") == 0 {
		paths = paths[1:]
		if len(paths) < 1 {
			//log
			return
		}
	} else {
		//log
		//w.WriteHeader(http.StatusBadRequest)
		//BUG: has no effect
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var a config.ArticlesType
	pa := c.Blog.Articles[:len(c.Blog.Articles)-2]
	for len(paths) != 0 {
	label:
		for _, article := range pa {
			if article.IsSame(paths[0]) {
				if len(paths) == 1 {
					if len(article.SubArticle) > 0 {
						a = append(a, article.SubArticle...)
					} else {
						a = append(a, article)
					}
				} else {
					pa = article.SubArticle
				}
				break label
			}
		}
		paths = paths[1:]
	}
	if len(a) == 0 {
		//log
		w.WriteHeader(http.StatusNotFound)
		return
	} else if len(a) == 1 {
		//tmp := blackfriday.MarkdownBasic(append([]byte("## "+a[0].Title+"\n"), a[0].Content...))
		tmp := blackfriday.Markdown(append([]byte("## "+a[0].Title+"\n"), a[0].Content...), blackfriday.HtmlRenderer(0|blackfriday.HTML_USE_XHTML, "", ""), blackfriday.EXTENSION_FENCED_CODE|blackfriday.EXTENSION_TABLES)

		a[0].Parse = string(tmp) //TODO: should Parse article only access it first .
		t, err := ttemplate.ParseFiles("themes/" + c.Blog.Theme + "/post.html")
		if err != nil {
			//log
			fmt.Println("t.ParseFiles occur some err: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		err = t.Execute(w, a[0])
		if err != nil {
			//log
			fmt.Println("t.Execute occur some err: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		t, err := ttemplate.ParseFiles("themes/" + c.Blog.Theme + "/index.html")
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
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Path
	fmt.Println(url)
	a := c.Blog.Articles[len(c.Blog.Articles)-2]
	tmp := blackfriday.Markdown(append([]byte("## "+a.Title+"\n"), a.Content...), blackfriday.HtmlRenderer(0|blackfriday.HTML_USE_XHTML, "", ""), blackfriday.EXTENSION_FENCED_CODE)

	a.Parse = string(tmp) //TODO: should Parse article only access it first .
	t, err := ttemplate.ParseFiles("themes/" + c.Blog.Theme + "/post.html")
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

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
}

//filepath should is a absPath
func newArticle(filePath string, fatherId string) (*config.ArticleType, error) {
	article := config.ArticleType{
		FilePath: filePath,
		Author:   c.Blog.Author,
		Tag:      config.FILE,
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
		article.Tag = config.DIR
		article.Title = fileInfo.Name()

		ieee := crc32.NewIEEE()
		ieee.Write([]byte(article.Title))
		s := strconv.FormatUint(uint64(ieee.Sum32()), 16)
		article.Id = s

		article.Url = "/post/" + article.Id

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
		slice := strings.Split(line, ": ") //FIXME: it's require new article must have header
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
		article.Title = strings.TrimRight(article.Title, ".md")
	}
	if strings.Compare(article.Id, "") == 0 {
		ieee := crc32.NewIEEE()
		ieee.Write([]byte(article.Title))
		s := strconv.FormatUint(uint64(ieee.Sum32()), 16)
		article.Id = s
	}
	if strings.Compare(article.Url, "") == 0 {
		//FIXME:the url is error when its in a directory
		if strings.Compare(fatherId, "") == 0 {
			article.Url = "/post/" + article.Id
		} else {
			article.Url = "/post/" + fatherId + "/" + article.Id
		}
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
