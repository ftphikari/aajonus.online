package main

import (
	"flag"
	"log"
	"net/http"
	"io/fs"
	"embed"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/ftphikari/teisai"
	"github.com/nanmu42/gzip"
)

//go:embed site
var site embed.FS

const (
	datefmt   = "2006-01-02"
	epochdate = "1970-01-01"
)

var (
	port  int
	// directory name, displayed name
	searchIndex = map[string]string{
		"articles": "Articles",
		"topics":   "Topics",
		"qna":      "Q&A",
	}
	epoch, _ = time.Parse(datefmt, epochdate)
	lock     = sync.RWMutex{}
	index    bleve.Index
	wg       sync.WaitGroup
	fsys     fs.FS
)

func textClean(text string) string {
	text = regexp.
		MustCompile(teisai.NormalImg).
		ReplaceAllString(text, " ")

	text = regexp.
		MustCompile(teisai.HiddenImg).
		ReplaceAllString(text, " ")

	text = regexp.
		MustCompile(teisai.SimpleLink).
		ReplaceAllString(text, " ")

	v := regexp.
		MustCompile(teisai.ComplexLink).
		FindAllStringSubmatch(text, -1)

	for i := range v {
		match, s1 := v[i][0], v[i][1]
		text = strings.Replace(text, match, s1, 1)
	}

	return text
}

func loadDirectory(dir string) {
	defer wg.Done()
	p, err := fs.ReadDir(fsys, dir)
	for err != nil {
		log.Println("ReadDir:", err)
		return
	}

	for _, f := range p {
		name := f.Name()
		fpath := filepath.Join(dir, name)
		if strings.HasPrefix(name, ".") {
			continue
		}
		if f.IsDir() {
			wg.Add(1)
			go loadDirectory(fpath)
			continue
		}

		b, err := fs.ReadFile(fsys, fpath)
		if err != nil {
			log.Printf("ReadFile(%s): %s\n", name, err)
			continue
		}

		text := teisai.ClearMetadata(string(b))

		data := struct {
			Text string
		}{
			textClean(text),
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := index.Index(fpath, data); err != nil {
				log.Println("loadDirectory:", err)
			}
		}()
	}
}

func loadCache() (time.Duration, error) {
	lock.Lock()
	defer lock.Unlock()

	ts := time.Now()

	// close if opened
	if index != nil {
		if err := index.Close(); err != nil {
			return 0, err
		}
	}

	// NOTE: MemOnly is actually SMALLER in RAM than regular index. Go figure!
	var err error
	index, err = bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		return 0, err
	}

	for s := range searchIndex {
		wg.Add(1)
		go loadDirectory(s)
	}
	wg.Wait()

	return time.Since(ts), nil
}

func reloadCache() {
	lt, err := loadCache()
	if err != nil {
		log.Println("Error loading cache:", err)
	} else {
		log.Println("Cache loaded in", lt)
	}
}

func main() {
	flag.IntVar(&port, "p", 8080, "port")
	flag.Parse()

	var err error
	fsys, err = fs.Sub(site, "site")
	if err != nil {
		log.Fatal("Unable to load embed site:", err)
	}

	http.Handle("/", gzip.DefaultHandler().WrapHandler(http.HandlerFunc(serve)))
	log.Println("Server started")
	reloadCache()
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(port), nil))
}
