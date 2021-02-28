package main

import (
	"bytes"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/formatters/html"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/styles"
	"github.com/dgraph-io/badger/v3"
	"github.com/forensicanalysis/gitfs"
	"github.com/google/codesearch/index"
	"github.com/google/codesearch/regexp"
)

type Index struct {
	path   string
	fileDB *badger.DB
}

func New(path string) (*Index, func() error, error) {
	_ = os.MkdirAll(path, fs.ModePerm)
	options := badger.DefaultOptions(filepath.Join(path, "badger"))
	// options.Logger = nil
	db, err := badger.Open(options)
	if err != nil {
		return nil, nil, err
	}

	return &Index{fileDB: db, path: path}, db.Close, nil
}

type searchResult struct {
	Count        int       `json:"count"`
	Repositories []string  `json:"repositories"`
	Snippets     []snippet `json:"snippets"`
}

type snippet struct {
	data []byte
	hits []int

	Repository string `json:"repo"`
	Path       string `json:"path"`
	Code       string `json:"code"`
	LineCount  int    `json:"line_count"`
	HitCount   int    `json:"hit_count"`
	FirstHit   int    `json:"first_hit"`
}

func (i Index) indexPath() string {
	return filepath.Join(i.path, "index.cs")
}

func (i *Index) search(offset, count int, repo, term string) (*searchResult, error) {
	log.Println("search", term)
	defer log.Println("search done")
	if _, err := os.Stat(i.indexPath()); err != nil {
		return nil, nil
	}

	snippets, repositories, err := i.loadSnippets(repo, term)
	if err != nil {
		return nil, err
	}

	snippetCount := len(snippets)
	snippets = limitSnippets(offset, count, snippets)
	snippets = extentSnippets(snippets)

	return &searchResult{Count: snippetCount, Repositories: keys(repositories), Snippets: snippets}, nil
}

func (i *Index) loadSnippets(repo, term string) ([]snippet, map[string]bool, error) {
	ix := index.Open(i.indexPath())
	re, err := regexp.Compile(term)
	if err != nil {
		return nil, nil, err
	}

	var snippets []snippet
	repositories := map[string]bool{}
	for _, fileid := range ix.PostingQuery(index.RegexpQuery(re.Syntax)) {
		gitRepo, gitPath := gitSplit(ix.Name(fileid))

		repositories[gitRepo] = true

		if repo != "" && repo != gitRepo {
			continue
		}

		err = i.fileDB.View(func(txn *badger.Txn) error {
			item, err := txn.Get([]byte(ix.Name(fileid)))
			if err != nil {
				return err
			}

			return item.Value(func(val []byte) error {
				hits := hits(val, re)
				if len(hits) == 0 {
					return nil
				}

				snippets = append(snippets, snippet{
					Repository: gitRepo,
					Path:       gitPath,
					hits:       hits,
					data:       val,
				})

				return nil
			})
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return snippets, repositories, nil
}

func gitSplit(p string) (repo, path string) {
	u, _ := url.Parse(p)
	parts := strings.Split(u.Path, "/")
	u.Path = "/" + strings.Join(parts[1:3], "/")
	return u.String(), strings.Join(parts[3:], "/")
}

func hits(data []byte, re *regexp.Regexp) (lineNumbers []int) {
	// re.Syntax.Flags |= syntax.FoldCase
	lastOffset := 0
	for {
		offset := re.Match(data[lastOffset:], false, false)
		if offset == -1 {
			break
		}
		lastOffset += offset
		lineNumbers = append(lineNumbers, lineNumber(data, lastOffset))
	}
	return lineNumbers
}

func lineNumber(data []byte, offset int) int {
	return bytes.Count(data[:offset], []byte("\n"))
}

func limitSnippets(offset int, count int, snippets []snippet) []snippet {
	snippetCount := len(snippets)
	start := offset
	if start > snippetCount {
		start = (snippetCount - 1) / 10
	}
	end := offset + count
	if end > snippetCount {
		end = snippetCount
	}
	return snippets[start:end]
}

func extentSnippets(snippets []snippet) []snippet {
	for idx := range snippets {
		s, err := format(snippets[idx].Path, snippets[idx].hits, string(snippets[idx].data))
		if err != nil {
			continue
		}
		snippets[idx].Code = s
		snippets[idx].LineCount = bytes.Count(snippets[idx].data, []byte{'\n'}) + 1
		snippets[idx].HitCount = len(snippets[idx].hits)
		snippets[idx].FirstHit = snippets[idx].hits[0]
	}
	return snippets
}

func format(path string, hits []int, data string) (string, error) {
	l := lexers.Get(path)
	if l == nil {
		l = lexers.Fallback
	}

	ranges := make([][2]int, len(hits))
	for idx, hit := range hits {
		ranges[idx] = [2]int{hit + 1, hit + 1}
	}

	l = chroma.Coalesce(l)
	f := html.New(
		html.HighlightLines(ranges),
		html.WithLineNumbers(true),
		html.WithClasses(true),
	)
	it, err := l.Tokenise(nil, data)
	if err != nil {
		return "", err
	}
	buf := &bytes.Buffer{}
	err = f.Format(buf, styles.VisualStudio, it)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func keys(repositories map[string]bool) (keys []string) {
	for key := range repositories {
		keys = append(keys, key)
	}
	return keys
}

func (i *Index) add(repoURL string) error {
	log.Println("add", repoURL)
	defer log.Println("add done")
	tmpIndexName := filepath.Join(i.path, "tmp.cs")
	if _, err := os.Stat(i.indexPath()); os.IsNotExist(err) {
		tmpIndexName = i.indexPath()
	}
	err := func() error {
		iw := index.Create(tmpIndexName)
		iw.AddPaths([]string{repoURL})
		defer iw.Flush()

		var fsys fs.FS
		fsys, err := gitfs.New(repoURL)
		if err != nil {
			return err
		}

		return i.fileDB.Update(func(txn *badger.Txn) error {
			return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
				if d.IsDir() || err != nil {
					return nil
				}

				b, err := fs.ReadFile(fsys, path)
				if err != nil {
					log.Println(err)
					return nil
				}

				err = txn.Set([]byte(repoURL+"/"+path), b)
				if err != nil {
					return err
				}

				iw.Add(repoURL+"/"+path, bytes.NewBuffer(b))
				return nil
			})
		})
	}()
	if err != nil {
		return err
	}

	if tmpIndexName != i.indexPath() {
		index.Merge(i.indexPath()+"~", i.indexPath(), tmpIndexName)
		err = os.Remove(tmpIndexName)
		if err != nil {
			return err
		}
		err = os.Rename(i.indexPath()+"~", i.indexPath())
		if err != nil {
			return err
		}
	}

	return nil
}
