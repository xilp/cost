package main

import (
	"io"
	"net/http"
	"strconv"
	"text/template"
)

func main() {
	t := NewTemplates("templates/", ".t", "notfound", "entry")
	a := NewEntrys()
	a.Run(8088, t)
}

type Entrys map[string]Entry

func NewEntrys() Entrys {
	p := make(Entrys)
	p.Load("")
	return p
}

func (p *Entrys) Run(port int, t *Template) {
	handle := func(w http.ResponseWriter, req *http.Request) {
		path := req.URL.Path[1:]
		entrys := *p
		entry, ok := entrys[path]
		if !ok {
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

func (p *Entrys) Load(file string) {
	entrys := *p
	entrys["test"] = Entry{"test", "标题"}
}

type Entry struct {
	path string
	title string
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







