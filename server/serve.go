package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/ftphikari/teisai"
)

type OGP struct {
	TabTitle string
	Title    string
	Desc     string
	Section  string
}

const PAGE404 = "<h1>PAGE NOT FOUND.</h1>"

func serveBase(w http.ResponseWriter, r *http.Request, page string, ogp OGP, query string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data, err := ioutil.ReadFile("base.htm")
	if err != nil {
		log.Println("serveBase:", err)
		return
	}

	t, err := template.New("base").Parse(string(data))
	if err != nil {
		log.Println("serveBase: parse template error:", err)
		return
	}

	if ogp.Title == "" {
		ogp.Title = "Aajonus Online"
		ogp.TabTitle = ogp.Title
	} else {
		ogp.TabTitle = ogp.Title + " - Aajonus Online"
	}
	if ogp.Desc == "" {
		ogp.Desc = "Online Aajonus Database"
	}
	url := "http://aajonus.online" + r.URL.String()

	st := struct {
		OGP    OGP
		Page   string
		Search string
		Magnet string
		URL    string
	}{
		ogp,
		page,
		query,
		magnet_link,
		url,
	}
	if err = t.ExecuteTemplate(w, "base", st); err != nil {
		log.Println("serveBase: execute template error:", err)
		return
	}
}

func serve404(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	var ogp OGP
	ogp.Title = "404"
	ogp.Desc = "Page not found."
	serveBase(w, r, PAGE404, ogp, "")
}

func getTitleDate(f string) (string, time.Time) {
	r, err := os.Open(f)
	if err != nil {
		log.Println("getTitleDate:", err)
		return "", epoch
	}

	metadata, ok := teisai.GetMetadataFromReader(r)
	if !ok {
		log.Println("getTitleDate: no metadata available")
		return "", epoch
	}

	dt, ok := metadata["date"]
	if !ok {
		dt = epochdate
	}
	date, err := time.Parse(datefmt, dt)
	if err != nil {
		log.Printf("Wrong date format for %s\n", f)
		date = epoch
	}

	title, ok := metadata["title"]
	if !ok {
		log.Printf("No title available for %s\n", f)
	}
	return title, date
}

func serveDir(w http.ResponseWriter, r *http.Request, p string) {
	p = strings.TrimSuffix(p, "/")

	var page string
	var ogp OGP

	files, err := ioutil.ReadDir(p)
	if err != nil {
		serve404(w, r)
		log.Println("serveDir:", err)
		return
	}

	t, ok := searchIndex[p]
	if !ok {
		ogp.Title = p
		ogp.Section = p

		page += "# " + p + "\n\n"
		for i := range files {
			link := filepath.Join("/", p, files[i].Name())
			page += "* @(" + link + ")\n"
		}
		page = teisai.RenderText(page)
		serveBase(w, r, page, ogp, "")
		return
	}

	ogp.Title = t
	ogp.Section = t

	page += "# " + t + "\n\n"

	t1 := time.Now().Add(time.Hour * -168) // 3 days
	for i := range files {
		link := filepath.Join("/", p, files[i].Name())
		link = strings.TrimSuffix(link, ".tei")

		title, date := getTitleDate(filepath.Join(p, files[i].Name()))
		page += `* @[` + title + "](" + link + ")"

		if t1.Before(date) {
			page += `<img src="/new.gif" style="width:31px;display:inline;">`
		}
		page += "\n"
	}
	page += "\n"

	page = teisai.RenderText(page)
	serveBase(w, r, page, ogp, "")
}

func servePage(w http.ResponseWriter, r *http.Request, f string) {
	data, err := ioutil.ReadFile(f)
	if err != nil {
		serve404(w, r)
		log.Println("servePage:", err)
		return
	}

	var ogp OGP
	ogp.Title = strings.TrimSuffix(filepath.Base(f), ".tei")

	dir, _ := filepath.Split(f)
	dir = path.Clean(dir)
	if section, ok := searchIndex[dir]; ok {
		ogp.Section = section
	}

	text := teisai.RenderText(string(data))
	if metadata, ok := teisai.GetMetadata(string(data)); ok {
		if t, ok := metadata["title"]; ok {
			ogp.Title = t
		}
		if d, ok := metadata["desc"]; ok {
			ogp.Desc = d
		}
	}

	serveBase(w, r, text, ogp, "")
}

func readUserIP(r *http.Request) (ip string) {
	ip = r.Header.Get("X-Real-Ip")
	if ip != "" {
		return
	}

	ip = r.Header.Get("X-Forwarded-For")
	if ip != "" {
		return
	}

	ip = r.RemoteAddr
	return
}

func serveSitemap(w http.ResponseWriter, r *http.Request) {
	keys := make([]string, 0, len(searchIndex))
	for k := range searchIndex {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	w.Write([]byte("https://aajonus.online\n"))
	for _, dir := range keys {
		w.Write([]byte("https://aajonus.online/" + dir + "\n"))
		d, err := ioutil.ReadDir(dir)
		for err != nil {
			log.Println("ReadDir:", err)
			return
		}
		for _, f := range d {
			name := f.Name()
			fpath := filepath.Join(dir, name)
			fpath = strings.TrimSuffix(fpath, ".tei")
			w.Write([]byte("https://aajonus.online/" + fpath + "\n"))
		}
	}
}

func serve(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		servePage(w, r, "index.tei")
		return
	}

	if p == "search" || strings.HasPrefix(p, "search/") {
		serveSearch(w, r)
		return
	}

	if p == "sitemap.txt" {
		serveSitemap(w, r)
		return
	}

	if f, err := os.Stat(p); err == nil {
		if f.IsDir() {
			serveDir(w, r, p)
			return
		}

		if strings.HasSuffix(p, ".css") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=604800")
		}
		http.ServeFile(w, r, p)
		return
	}

	if _, err := os.Stat(p + ".tei"); err != nil {
		serve404(w, r)
		log.Println("serve:", p, "not found.", readUserIP(r))
		return
	}

	servePage(w, r, p+".tei")
}
