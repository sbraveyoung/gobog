package search

import (
	"os"

	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/mapping"
	"github.com/yanyiwu/gojieba"
	_ "github.com/yanyiwu/gojieba/bleve"
)

const (
	INDEX_DIR = "_source/search.bleve"
)

type Engine interface {
	Cut(url, s string) error
	Search(query []string) []string
}

type engineImpl struct {
	jieba   *gojieba.Jieba
	mapping *mapping.IndexMappingImpl
	index   bleve.Index
}

func New() (Engine, error) {
	e := engineImpl{
		jieba:   gojieba.NewJieba(),
		mapping: bleve.NewIndexMapping(),
	}

	err := os.RemoveAll(INDEX_DIR)
	if err != nil {
		return e, err
	}
	err = e.mapping.AddCustomTokenizer("gojieba",
		map[string]interface{}{
			"dictpath":     gojieba.DICT_PATH,
			"hmmpath":      gojieba.HMM_PATH,
			"userdictpath": gojieba.USER_DICT_PATH,
			"idf":          gojieba.IDF_PATH,
			"stop_words":   gojieba.STOP_WORDS_PATH,
			"type":         "gojieba",
		},
	)
	if err != nil {
		return e, err
	}
	err = e.mapping.AddCustomAnalyzer("gojieba",
		map[string]interface{}{
			"type":      "gojieba",
			"tokenizer": "gojieba",
		})
	if err != nil {
		return e, err
	}
	e.mapping.DefaultAnalyzer = "gojieba"
	e.index, err = bleve.New(INDEX_DIR, e.mapping)
	if err != nil {
		return e, err
	}
	return e, nil
}

func (e engineImpl) Cut(url, s string) error {
	words := e.jieba.CutForSearch(s, true)
	err := e.index.Index(url, words)
	if err != nil {
		return err
	}
	return nil
}

func (e engineImpl) Search(querys []string) (urls []string) {
	for _, query := range querys {
		req := bleve.NewSearchRequest(bleve.NewQueryStringQuery(query))
		req.Highlight = bleve.NewHighlight()
		res, err := e.index.Search(req)
		if err != nil {
			//log err
			continue
		}
		for _, item := range res.Hits {
			urls = append(urls, item.ID)
		}
	}
	return urls
}
