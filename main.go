package main

import (
	"crypto/tls"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

//go:generate yarn --cwd ui install
//go:generate yarn --cwd ui build
func main() {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	// disable logging
	log.SetOutput(os.Stdout)

	dbPath := os.Getenv("CODESEARCH_VOLUME")
	if dbPath == "" {
		dbPath = filepath.Join(".", "db")
	}

	index, indexClose, err := New(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer indexClose()

	// index initial repostories
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if strings.HasPrefix(pair[0], "CODESEARCH_REPO_") {
			err := index.add(pair[1])
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	// setup webserver
	if os.Getenv("CODESEARCH_DEV") == "true" {
		http.HandleFunc("/", proxy)
	} else {
		http.HandleFunc("/", static)
	}
	http.HandleFunc("/search", searchHandler(index))
	http.HandleFunc("/index", indexHandler(index))
	log.Fatal(http.ListenAndServe(":8000", nil))
}

//go:embed ui/dist/*
var ui embed.FS

func static(w http.ResponseWriter, req *http.Request) {
	fsys, _ := fs.Sub(ui, "ui/dist")
	http.FileServer(http.FS(fsys)).ServeHTTP(w, req)
}

func proxy(w http.ResponseWriter, req *http.Request) {
	u, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(u)

	req.Host = req.URL.Host

	proxy.ServeHTTP(w, req)
}

type indexRequest struct {
	URL string `json:"url"`
}

func indexHandler(index *Index) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "Invalid method.", http.StatusMethodNotAllowed)
			return
		}

		var p indexRequest
		if err := json.NewDecoder(request.Body).Decode(&p); err != nil {
			http.Error(writer, "No valid url given.", http.StatusBadRequest)
			return
		}

		if _, err := url.Parse(p.URL); p.URL != "" && err != nil {
			http.Error(writer, "No valid url given.", http.StatusBadRequest)
			return
		}

		if err := index.add(p.URL); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	}
}

func searchHandler(index *Index) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(writer, "Invalid method.", http.StatusMethodNotAllowed)
			return
		}

		q := request.URL.Query().Get("q")
		if q == "" {
			http.Error(writer, "No query given.", http.StatusBadRequest)
			return
		}

		start, err := strconv.Atoi(request.URL.Query().Get("offset"))
		if err != nil {
			start = 0
		}

		repo := request.URL.Query().Get("repo")

		results, err := index.search(start, 10, repo, q)
		if err != nil {
			http.Error(writer, "Search error: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := json.NewEncoder(writer).Encode(results); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	}
}
