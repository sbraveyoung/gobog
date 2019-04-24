package article

import (
	"bufio"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SmartBrave/gobog/pkg/config"
	"github.com/astaxie/beego/logs"
)

const (
	FILE = iota
	DIR
)

func NewArticle(filePath string, author string, fatherId ...string) (ArticleType, error) {
	article := ArticleType{
		FilePath: filePath,
		Author:   author,
		Tag:      FILE,
	}
	if len(fatherId) != 0 && len(fatherId) != 1 {
		return article, errors.New(fmt.Sprintf("fatherId is err. len(fatherId):%d", len(fatherId)))
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return article, err
	}

	sysInfo := fileInfo.Sys()
	if stat, ok := sysInfo.(*syscall.Stat_t); ok {
		mTime := time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec) //Mtime,because we can't get create time
		article.ModifyTime = mTime.Unix()
	}

	if fileInfo.IsDir() {
		article.Tag = DIR
		article.Title = fileInfo.Name()

		ieee := crc32.NewIEEE()
		ieee.Write([]byte(article.Title))
		s := strconv.FormatUint(uint64(ieee.Sum32()), 16)
		article.Id = s

		article.Url = fmt.Sprintf("/%s/%s", config.DIRS["posts"], article.Id)

		return article, nil
	}

	file, err := os.OpenFile(filePath, os.O_RDWR, 0666)
	if err != nil {
		return article, err
	}

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return article, err
		}

		if strings.HasPrefix(line, "---end---") {
			items := strings.Split(string(article.Content), "\n")
			items = items[:len(items)-1]
			for _, item := range items {
				slice := strings.Split(item, ": ")
				if len(slice) != 2 {
					logs.Warn(article.Title, " has error header:", item)
					continue
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
					logs.Warn(article.Title, " has error header:", item)
					continue
				}
			}
			article.Content = []byte{}
		} else {
			article.Content = append(article.Content, []byte(line)...)
		}
	}

	if strings.Compare(article.Title, "") == 0 {
		article.Title = fileInfo.Name()
		article.Title = strings.TrimRight(article.Title, ".md")
	}
	if strings.Compare(article.Id, "") == 0 {
		ieee := crc32.NewIEEE()
		ieee.Write([]byte(article.Title))
		article.Id = strconv.FormatUint(uint64(ieee.Sum32()), 16)
	}
	if strings.Compare(article.Url, "") == 0 {
		if len(fatherId) != 0 {
			article.Url = fmt.Sprintf("/%s/%s/%s", config.DIRS["posts"], fatherId[0], article.Id)
		} else {
			article.Url = fmt.Sprintf("/%s/%s", config.DIRS["posts"], article.Id)
		}
	}
	if strings.Compare(article.Time, "") == 0 {
		article.Time = time.Now().Format("2006-01-02 15:04:05")
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		return article, err
	}
	//Future: could not write every time if this article is not publish first.
	writeString := []byte{}
	writer := bufio.NewWriter(file)
	writeString = append(writeString, []byte("title: "+article.Title+"\n")...)
	writeString = append(writeString, []byte("author: "+article.Author+"\n")...)
	writeString = append(writeString, []byte("date: "+article.Time+"\n")...)
	writeString = append(writeString, []byte("url: "+article.Url+"\n")...)
	writeString = append(writeString, []byte("id: "+article.Id+"\n")...)
	writeString = append(writeString, []byte("---end---\n")...)
	writeString = append(writeString, article.Content...)
	_, err = writer.WriteString(string(writeString))
	if err != nil {
		return article, err
	}
	err = writer.Flush()
	if err != nil {
		return article, err
	}
	return article, nil
}
