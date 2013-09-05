package main

import (
	"io"
	"io/ioutil"
	"os"
	"net/http"
	"strconv"
	"text/template"
)

func main() {
	t := NewTemplates("templates/", ".tp", "notfound", "entry")
	Run(8088, "entrys/", ".sp", t)
}

func Run(port int, dir string, ext string, t *Template) {
	handle := func(w http.ResponseWriter, req *http.Request) {
		path := req.URL.Path[1:]
		entry := LoadEntry(dir + path + ext)
		if entry == nil {
			t.Rend(w, "notfound", nil)
		} else {
			t.Rend(w, "entry", entry)
		}
	}

	http.HandleFunc("/", handle)
	err := http.ListenAndServe(":" + strconv.Itoa(port), nil)
	if err != nil {
		panic(err)
	}
}

type Entry struct {
	content string
}

func LoadEntry(path string) *Entry {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	content, err := ioutil.ReadAll(file)
	if err != nil {
		return nil
	}
	return &Entry{string(content)}
}

type Template struct {
	templ *template.Template
	ext string
}

func NewTemplates(dir string, ext string, files ...string) *Template {
	paths := make([]string, len(files))
	for i, it := range files {
		paths[i] = dir + it + ext
	}
	p := &Template{template.New(""), ext}
	_, err := p.templ.ParseFiles(paths...)
	if err != nil {
		panic(err)
	}
	return p
}

func (p *Template) Rend(w io.Writer, name string, data interface{}) {
	err := p.templ.ExecuteTemplate(w, name + p.ext, data)
	if err != nil {
		panic(err)
	}
}







