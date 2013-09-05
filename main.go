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
	debug := !(len(os.Args) > 1 && os.Args[1] == "release")
	Run(8088, "entrys/", ".sp", t, debug)
}

func Run(port int, dir string, ext string, t *Template, debug bool) {
	handle := func(w http.ResponseWriter, req *http.Request) {
		path := req.URL.Path[1:]
		entry := LoadEntry(dir + path + ext)
		if debug {
			t.Load()
		}
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
	Content string
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
	dir string
	ext string
	files []string
}

func NewTemplates(dir string, ext string, files ...string) *Template {
	p := &Template{nil, dir, ext, files}
	p.Load()
	return p
}

func (p *Template) Load() {
	p.templ = template.New("")
	paths := make([]string, len(p.files))
	for i, it := range p.files {
		paths[i] = p.dir + it + p.ext
	}
	_, err := p.templ.ParseFiles(paths...)
	if err != nil {
		panic(err)
	}
}

func (p *Template) Rend(w io.Writer, name string, data interface{}) {
	err := p.templ.ExecuteTemplate(w, name + p.ext, data)
	if err != nil {
		panic(err)
	}
}
