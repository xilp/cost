package main

import (
	"bufio"
	"flag"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/template"
)

func main() {
	templates := flag.String("t", "templates", "template files dir")
	entrys := flag.String("e", "entrys", "entry files dir")
	debug := flag.Bool("d", false, "debug mode")
	port := flag.Int("p", 8088, "port")
	flag.Parse()

	render := NewTemplates(*templates, ".htm", "notfound", "entry")
	err := NewEntrys().Run(*debug, *port, *entrys, ".sp", render, []string{".htm", ".html"})
	if err != nil {
		println(err.Error())
	}
}

func NewEntrys() *Entrys {
	return &Entrys{NewEntryCache()}
}

func (p *Entrys) Run(debug bool, port int, dir string, ext string, t *Template, suffix []string) error {
	parse := func(url string) (string, string) {
		path := url[1:]
		for _, it := range suffix {
			if strings.HasSuffix(strings.ToLower(path), it) {
				path = path[:len(path)-len(it)]
				break
			}
		}
		i := strings.LastIndex(path, "/")
		if i > 0 {
			return path[:i], path[i+1:]
		}
		return path, ""
	}

	handle := func(w http.ResponseWriter, req *http.Request) {
		path, op := parse(req.URL.Path)
		path = dir + "/" + path + ext

		var entry *Entry
		if debug {
			entry = LoadEntry(path)
			t.Load()
		} else {
			entry = p.cache.Load(path)
		}

		switch op {
		case "update":
			p.cache.Discard(path)
		default:
			if entry == nil {
				t.Rend(w, "notfound", nil)
			} else {
				t.Rend(w, "entry", entry)
			}
		}
	}

	http.HandleFunc("/", handle)
	return http.ListenAndServe(":"+strconv.Itoa(port), nil)
}

type Entrys struct {
	cache *EntryCache
}

func NewEntryCache() *EntryCache {
	return &EntryCache{make(map[string]*Entry)}
}

func (p *EntryCache) Load(path string) *Entry {
	entry, ok := p.entrys[path]
	if ok {
		return entry
	}
	entry = LoadEntry(path)
	p.entrys[path] = entry
	return entry
}

func (p *EntryCache) Discard(path string) {
	delete(p.entrys, path)
}

type EntryCache struct {
	entrys map[string]*Entry
}

func LoadEntry(path string) *Entry {
	entry := &Entry{}
	tagi := map[string]int{}
	useri := map[string]int{}

	raise := func(msg interface{}, row int) {
	}

	tagf := func(line string, row int) {
		segs := strings.Fields(line)
		if len(segs) != 2 {
			raise("parse tag line error", row)
		}
		idx, tag := segs[0], segs[1]
		tagi[idx] = len(entry.Tags)
		entry.Tags = append(entry.Tags, tag)
	}

	userf := func(line string, row int) {
		segs := strings.Fields(line)
		if len(segs) != 2 {
			raise("parse user line error", row)
		}
		idx, user := segs[0], segs[1]
		useri[idx] = len(entry.Users)
		entry.Users = append(entry.Users, user)
	}

	tagsf := func(line string, row int) []int {
		line = strings.TrimSpace(line)
		if line[0] != '{' || line[len(line)-1] != '}' {
			raise("parse tags error", row)
		}
		line = line[1 : len(line)-1]
		strs := strings.Split(line, ",")
		tags := make([]int, len(strs))
		for i, str := range strs {
			tag, ok := tagi[str]
			if !ok {
				raise("parse tag error", row)
			}
			tags[i] = tag
		}
		return tags
	}

	costf := func(line string, row int) Cost {
		segs := strings.Fields(line)
		if len(segs) != 3 {
			raise("parse cost line error", row)
		}
		price, err := strconv.Atoi(segs[0])
		if err != nil {
			raise("parse price error", row)
		}
		user, ok := useri[segs[1]]
		if !ok {
			raise("parse user error", row)
		}
		tags := tagsf(segs[2], row)
		return Cost{price, user, tags}
	}

	unScopedf := func(line string, row int) {
		if len(entry.Title) == 0 {
			entry.Title = line
		} else {
			entry.Costs = append(entry.Costs, costf(line, row))
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	Scopes := NewScopes(unScopedf)
	Scopes.Add("{", "}", tagf)
	Scopes.Add("[", "]", userf)
	Scopes.Parse(file)

	return entry
}

type Entry struct {
	Title string
	Tags  []string
	Users []string
	Costs []Cost
}
type Cost struct {
	Price int
	User  int
	Tags  []int
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
		paths[i] = p.dir + "/" + it + p.ext
	}
	_, err := p.templ.ParseFiles(paths...)
	if err != nil {
		panic(err)
	}
}

func (p *Template) Rend(w io.Writer, name string, data interface{}) {
	err := p.templ.ExecuteTemplate(w, name+p.ext, data)
	if err != nil {
		panic(err)
	}
}

type Template struct {
	templ *template.Template
	dir   string
	ext   string
	files []string
}

func NewScopes(unScoped Parser) *Scopes {
	return &Scopes{
		unScoped,
		make([]*Scope, 0),
		make(map[string]*Scope),
	}
}

func (p *Scopes) Add(begin string, end string, fun Parser) {
	Scope := &Scope{begin, end, fun}
	p.list = append(p.list, Scope)
	p.set[begin] = Scope
}

func (p *Scopes) Parse(file io.Reader) {
	reader := bufio.NewReader(file)
	stack := make([]*Scope, 0)
	for i := 0; ; i++ {
		data, prefix, err := reader.ReadLine()
		if prefix {
			panic("buffer too small")
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		line := strings.TrimSpace(string(data))
		if Scope, ok := p.set[line]; ok {
			stack = append(stack, Scope)
		} else if len(stack) == 0 {
			p.unScoped(line, i)
		} else {
			last := stack[len(stack)-1]
			if line == last.end {
				stack = stack[:len(stack)-1]
			} else {
				last.fun(line, i)
			}
		}
	}
}

type Scopes struct {
	unScoped Parser
	list     []*Scope
	set      map[string]*Scope
}
type Scope struct {
	begin string
	end   string
	fun   Parser
}
type Parser func(line string, row int)
