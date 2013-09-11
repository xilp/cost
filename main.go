package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/template"
)

func main() {
	admin := flag.String("a", "1234", "admin password")
	templates := flag.String("t", "templates", "template files dir")
	entrys := flag.String("e", "entrys", "entry files dir")
	debug := flag.Bool("d", false, "debug mode")
	port := flag.Int("p", 8088, "port")
	flag.Parse()

	db := NewFileDB(*entrys, ".sp")
	render := NewTemplates(*templates, ".htm", "404", "entry")
	err := NewEntrys().Run(*admin, *debug, *port, db, render, []string{".htm", ".html"})
	if err != nil {
		println(err.Error())
	}
}

func NewEntrys() *Entrys {
	return &Entrys{NewEntryCache(), NewEntryWriter(), NewLockers()}
}

func (p *Entrys) Run(admin string, debug bool, port int, db DB, t *Template, suffix []string) error {
	parse := func(url string) (string, string) {
		id := url[1:]
		if id[0] == '_' {
			return "", id[1:]
		}
		for _, it := range suffix {
			if strings.HasSuffix(strings.ToLower(id), it) {
				id = id[:len(id) - len(it)]
				break
			}
		}
		i := strings.LastIndex(id, "/")
		if i > 0 {
			return id[:i], id[i + 1:]
		}
		return id, ""
	}

	load := func(id string) *Entry {
		if debug {
			entry := LoadEntry(id, db)
			t.Load()
			return entry
		} else {
			return p.cache.Load(id, db)
		}
	}

	write := func(id string, entry *Entry, req *http.Request) {
		req.ParseForm()
		get := func(key string) string {
			val := req.Form[key]
			if len(val) == 0 {
				return ""
			}
			return val[0]
		}

		title := get("title")
		price, err := strconv.Atoi(get("price"))
		if err != nil {
			panic(err)
		}
		tags := strings.Split(get("tags"), ",")
		user := get("user")

		if debug {
			WriteEntry(id, db, entry, title, price, tags, user)
		} else {
			p.writer.Write(id, db, entry, title, price, tags, user)
		}
	}

	sysop := func(w http.ResponseWriter, req *http.Request, op string) {
		req.ParseForm()
		if len(req.Form["phrase"]) == 0 || admin != req.Form["phrase"][0] {
			w.Write([]byte("wrong phrase"))
			return
		}
		switch op {
		case "exit":
			if len(req.Form["phrase"]) > 0 && admin == req.Form["phrase"][0] {
				os.Exit(0)
			}
		}
	}
	
	valid := func(id string) bool {
		return true
	}

	invoke := func(w http.ResponseWriter, req *http.Request) {
		id, op := parse(req.URL.Path)
		if len(id) == 0{
			sysop(w, req, op)
			return
		}
		if !valid(id) {
			panic("invalid id")
		}

		p.locker.Lock(id)
		defer p.locker.Unlock(id)

		switch op {
		case "update":
			p.cache.Discard(id)
			w.Write([]byte("ok"))
		case "edit":
			entry := load(id)
			write(id, entry, req)
			w.Write([]byte("ok"))
		default:
			entry := load(id)
			if entry == nil {
				t.Rend(w, "404", nil)
			} else {
				t.Rend(w, "entry", entry)
			}
		}
	}

	handle := func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			if debug {
				return
			}
			if err := recover(); err != nil {
				println(fmt.Sprintf("%v", err))
			}
		}()
		invoke(w, req)
	}

	http.HandleFunc("/", handle)
	return http.ListenAndServe(":" + strconv.Itoa(port), nil)
}

type Entrys struct {
	cache *EntryCache
	writer *EntryWriter
	locker *Lockers
}

func NewEntryCache() *EntryCache {
	return &EntryCache{entrys:make(map[string]*Entry)}
}

func (p *EntryCache) Load(id string, db DB) *Entry {
	entry, ok := p.entrys[id]
	if ok {
		return entry
	}
	entry = LoadEntry(id, db)
	p.entrys[id] = entry
	return entry
}

func (p *EntryCache) Discard(id string) {
	delete(p.entrys, id)
}

type EntryCache struct {
	entrys map[string]*Entry
	lock sync.Mutex
}

func NewEntryWriter() *EntryWriter {
	return &EntryWriter{}
}

func (p *EntryWriter) Write(id string, db DB, entry *Entry, title string, price int, tags []string, user string) {
	WriteEntry(id, db, entry, title, price, tags, user)
}

type EntryWriter struct {
}

func LoadEntry(id string, db DB) *Entry {
	entry := &Entry{}
	tagi := 0
	useri := 0
	row := 0

	raise := func(msg string) {
		panic(msg + " #" + id + ":" + strconv.Itoa(row + 1))
	}

	atoi := func(s string) int {
		n, err := strconv.Atoi(s)
		if err != nil {
			raise(err.Error())
		}
		return n
	}

	tagf := func(line string) {
		segs := strings.Fields(line)
		if len(segs) != 2 {
			raise("parse tag line error")
		}
		idx, tag := segs[0], segs[1]
		if tagi != atoi(idx) || tagi != len(entry.Tags) {
			raise("tag index error")
		}
		entry.Tags = append(entry.Tags, tag)
		tagi += 1
	}

	userf := func(line string) {
		segs := strings.Fields(line)
		if len(segs) != 2 {
			raise("parse user line error")
		}
		idx, user := segs[0], segs[1]
		if useri != atoi(idx) || useri != len(entry.Users) {
			raise("user index error")
		}
		entry.Users = append(entry.Users, user)
		useri += 1
	}

	tagsf := func(line string) []int {
		strs := strings.Split(line, ",")
		tags := make([]int, len(strs))
		for i, it := range strs {
			tags[i] = atoi(it)
		}
		return tags
	}

	costf := func(line string) Cost {
		segs := strings.Fields(line)
		if len(segs) != 3 {
			raise("parse cost line error")
		}
		return Cost{atoi(segs[0]), atoi(segs[1]), tagsf(segs[2])}
	}

	file := db.Get(id)
	if file == nil {
		return nil
	}
	defer file.Close()
	reader := bufio.NewReader(file)

	for ; ; row++ {
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
		if len(data) == 0 {
			continue
		}
		line := strings.TrimSpace(string(data))

		switch line[0] {
		case '#':
			entry.Title = line[1:]
		case '$':
			tagf(line[1:])
		case '*':
			userf(line[1:])
		default:
			entry.Costs = append(entry.Costs, costf(line))
		}
	}
	return entry
}

func WriteEntry(id string, db DB, entry *Entry, title string, price int, tags []string, user string) {
	write := func(data []byte) {
		file := db.Append(id)
		defer file.Close()
		_, err := file.Write(data)
		if err != nil {
			panic(err)
		}
	}

	str := strconv.Itoa
	strs := func(ints []int) []string {
		r := make([]string, len(ints))
		for i, it := range ints {
			r[i] = str(it)
		}
		return r
	}

	buf := new(bytes.Buffer)

	if entry == nil {
		if len(title) == 0 {
			panic("must have title")
		}
		entry = &Entry{}
	}

	if len(title) != 0 {
		if title[0] == '#' {
			panic("# not allow")
		}
		buf.WriteString("#" + title + "\n")
	}

	cost := Cost{}
	cost.User = -1
	for i, it := range entry.Users {
		if it == user {
			cost.User = i
		}
	}
	if cost.User < 0 {
		idx := len(entry.Users)
		cost.User = idx
		entry.Users = append(entry.Users, user)
		buf.WriteString("*" + str(idx) + " " + user + "\n")
	}

	tagi := map[string]int{}
	for i, it := range entry.Tags {
		tagi[it] = i
	}

	newt := map[string]int{}
	for _, it := range tags {
		if i, ok := tagi[it]; ok {
			cost.Tags = append(cost.Tags, i)
		} else {
			idx := len(entry.Tags)
			cost.Tags = append(cost.Tags, idx)
			entry.Tags = append(entry.Tags, it)
			newt[it] = idx
		}
	}
	for k, v := range newt {
		buf.WriteString("$" + str(v) + " " + k + "\n")
	}

	entry.Costs = append(entry.Costs, cost)
	buf.WriteString(str(price) + " " + str(cost.User) + " ")
	buf.WriteString(" " + strings.Join(strs(cost.Tags), ",") + "\n")

	write(buf.Bytes())
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

func NewFileDB(dir string, ext string) DB {
	return &FileDB{dir, ext}
}

func (p *FileDB) Get(id string) io.ReadCloser {
	path := p.dir + "/" + id + p.ext
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		panic(err)
	}
	return file
}

func (p *FileDB) Append(id string) io.WriteCloser {
	path := p.dir + "/" + id + p.ext
	file, err := os.OpenFile(path, os.O_RDWR | os.O_APPEND | os.O_CREATE, 0660)
	if err != nil {
		panic(err)
	}
	return file
}

type FileDB struct {
	dir string
	ext string
}

type DB interface {
	Get(id string) io.ReadCloser
	Append(id string) io.WriteCloser
}

func NewLockers() *Lockers {
	return &Lockers{
		lockeds: make(map[string]int),
		lockers: make([]*sync.Mutex, 0),
	}
}

type Lockers struct {
	lockeds map[string]int
	lockers []*sync.Mutex
	locker sync.Mutex
}

func (p *Lockers) Lock(id string) {
	get := func() *sync.Mutex {
		p.locker.Lock()
		defer p.locker.Unlock()
		idx, ok := p.lockeds[id]
		if !ok {
			idx = len(p.lockers)
			p.lockers = append(p.lockers, new(sync.Mutex))
			p.lockeds[id] = idx
		}
		return p.lockers[idx]
	}
	get().Lock()
}

func (p *Lockers) Unlock(id string) {
	get := func() *sync.Mutex {
		p.locker.Lock()
		defer p.locker.Unlock()
		idx, _ := p.lockeds[id]
		return p.lockers[idx]
	}
	get().Unlock()
}

