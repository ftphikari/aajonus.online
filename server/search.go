package main

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/blevesearch/bleve/v2"
	h "github.com/blevesearch/bleve/v2/search/highlight/highlighter/html"
	"github.com/ftphikari/teisai"
)

func search(q string) string {
	lock.RLock()
	defer lock.RUnlock()

	page := ""

	query := bleve.NewQueryStringQuery(q)
	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Highlight = bleve.NewHighlightWithStyle(h.Name)
	searchRequest.Size = 200

	if index == nil {
		page = "<h1>Search error</h1>"
		return page
	}

	sr, err := index.Search(searchRequest)
	if err != nil {
		fmt.Println(err)
		page = "<h1>Search error</h1>"
		return page
	}

	if sr.Total <= 0 {
		page = "<h1>No results</h1>"
		return page
	}

	//	page = fmt.Sprintf("<h1>%d matches, showing %d through %d, took %s</h1>\n", sr.Total, sr.Request.From+1, sr.Request.From+len(sr.Hits), sr.Took)
	for _, hit := range sr.Hits {
		r, err := os.Open(hit.ID)
		if err != nil {
			log.Println(err)
			continue
		}

		name := strings.TrimSuffix(hit.ID, ".tei")
		link := filepath.Join("/", name)

		if metadata, ok := teisai.GetMetadataFromReader(r); ok {
			if title, ok := metadata["title"]; ok {
				name = title
			}
		}

		page += fmt.Sprintf(`<h2><a href="%s">%s</a></h2>`+"\n", link, name)
		for _, fragments := range hit.Fragments {
			for _, fragment := range fragments {
				page += fmt.Sprintf("<blockquote>\n%s\n</blockquote>\n", fragment)
			}
		}
	}

	return page
}

func serveSearch(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	q := r.Form.Get("q")
	q = regexp.
		MustCompile(`[^a-zA-Z0-9"\\+-]+`).
		ReplaceAllString(q, " ")
	q = regexp.MustCompile(`\s+`).
		ReplaceAllString(q, " ")
	q = strings.TrimSpace(q)

	page := ""
	if len(q) < 3 {
		page = "<h1>Query is too short</h1>"
	} else if len(q) > 33 {
		page = "<h1>Query is too long</h1>"
	} else {
		page = search(q)
	}

	var ogp OGP
	ogp.Title = q
	ogp.Desc = "Search results"

	serveBase(w, r, page, ogp, html.EscapeString(q))
}
