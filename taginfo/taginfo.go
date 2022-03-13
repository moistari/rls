// Package taginfo contains tag info.
package taginfo

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// columns is the columns count.
const columns = 6

// RegisterType registers release types.
func RegisterType(typ string, i int) {
	types[typ] = i
}

// types is the map of string types.
var types map[string]int

func init() {
	types = make(map[string]int)
}

// Taginfo describes tag info.
type Taginfo struct {
	tag    string
	title  string
	regexp string
	other  string
	typ    int
	excl   bool
	re     *regexp.Regexp
}

// New creates a new tag info.
func New(strs ...string) (*Taginfo, error) {
	if len(strs) != columns {
		return nil, fmt.Errorf("tag info must have %d fields", columns)
	}
	tag, title, re, other, typstr, excl := strs[0], strs[1], strs[2], strs[3], strs[4], strs[5] == "1"
	typ, ok := types[typstr]
	switch {
	case tag == "":
		return nil, errors.New("must define tag")
	case title == "":
		title = tag
	case !ok:
		return nil, fmt.Errorf("invalid type %q", typstr)
	}
	info := &Taginfo{
		tag:    tag,
		title:  title,
		regexp: re,
		other:  other,
		excl:   excl,
		typ:    typ,
	}
	var err error
	if info.re, err = regexp.Compile(`(?i)^(?:` + info.RE() + `)$`); err != nil {
		return nil, fmt.Errorf("tag %q has invalid regexp %q", tag, re)
	}
	return info, nil
}

// Load loads a set of csv tag info from the reader.
func Load(rdr io.Reader) (map[string][]*Taginfo, error) {
	r := csv.NewReader(rdr)
	// check header
	switch v, err := r.Read(); {
	case err != nil && errors.Is(err, io.EOF):
		return nil, errors.New("empty csv")
	case err != nil:
		return nil, err
	case len(v) != columns+1:
		return nil, fmt.Errorf("must have %d columns, got %q", columns+1, v)
	case v[0] != "Type" || v[1] != "Tag" || v[2] != "Title" || v[3] != "Regexp" || v[4] != "Other" || v[5] != "ReleaseType" || v[6] != "TypeExclusive":
		return nil, errors.New("must have csv headers Type, Tag, Title, Regexp, Other, ReleaseType, TypeExclusive")
	}
	m, exists := make(map[string][]*Taginfo), make(map[string]map[string]int64)
	for i := int64(0); ; i++ {
		// read tag info
		v, err := r.Read()
		switch {
		case err != nil && errors.Is(err, io.EOF):
			return m, nil
		case err != nil:
			return nil, err
		}
		if exists[v[0]] == nil {
			exists[v[0]] = make(map[string]int64)
		}
		if n, ok := exists[v[0]][v[1]]; ok {
			return nil, fmt.Errorf("line %d: type %q with tag %q previously defined on line %d", i+1, v[0], v[1], n+1)
		}
		info, err := New(v[1:]...)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		m[v[0]] = append(m[v[0]], info)
		exists[v[0]][v[1]] = i
	}
}

// LoadFile loads csv tag info from a file.
func LoadFile(file string) (map[string][]*Taginfo, error) {
	f, err := os.OpenFile(file, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(file), err)
	}
	infos, err := Load(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(file), err)
	}
	return infos, nil
}

// LoadBytes loads csv tag info from buf.
func LoadBytes(buf []byte) (map[string][]*Taginfo, error) {
	return Load(bytes.NewReader(buf))
}

// LoadAll loads all embedded tag info.
func LoadAll() (map[string][]*Taginfo, error) {
	return LoadBytes(taginfoCsv)
}

// Must creates a new tag info.
func Must(strs ...string) *Taginfo {
	info, err := New(strs...)
	if err != nil {
		panic(err)
	}
	return info
}

// MustLoadFile loads csv tag info from a file.
func MustLoadFile(file string) map[string][]*Taginfo {
	infos, err := LoadFile(file)
	if err != nil {
		panic(err)
	}
	return infos
}

// MustLoadBytes loads csv tag info from buf.
func MustLoadBytes(buf []byte) map[string][]*Taginfo {
	infos, err := LoadBytes(buf)
	if err != nil {
		panic(err)
	}
	return infos
}

// All loads all embedded tag info and any passed extras.
func All(extras ...map[string][]*Taginfo) map[string][]*Taginfo {
	infos, err := LoadAll()
	if err != nil {
		panic(err)
	}
	for _, extra := range extras {
		for k, v := range extra {
			infos[k] = append(infos[k], v...)
		}
	}
	return infos
}

// Tag returns the tag info tag.
func (info *Taginfo) Tag() string {
	return info.tag
}

// Title returns the tag info title.
func (info *Taginfo) Title() string {
	return info.title
}

// Regexp returns the tag info regexp.
func (info *Taginfo) Regexp() string {
	return info.regexp
}

// Other returns the tag info other.
func (info *Taginfo) Other() string {
	return info.other
}

// Type returns the tag info type.
func (info *Taginfo) Type() int {
	return info.typ
}

// Excl returns the tag info excl.
func (info *Taginfo) Excl() bool {
	return info.excl
}

// RE returns the tag info regexp string.
func (info *Taginfo) RE() string {
	if info.regexp != "" {
		return info.regexp
	}
	return `\Q` + info.tag + `\E`
}

// Match matches the tag info to s.
func (info *Taginfo) Match(s string) bool {
	return info.re.MatchString(s)
}

// FindFunc is the find signature..
type FindFunc func(string) *Taginfo

// Find returns a func to find tag info.
func Find(infos ...*Taginfo) FindFunc {
	n := len(infos)
	return func(s string) *Taginfo {
		for i := 0; i < n; i++ {
			if infos[i].Match(s) {
				return infos[i]
			}
		}
		return nil
	}
}

// AllBytes returns embedded tag info in csv format.
func AllBytes() []byte {
	buf := make([]byte, len(taginfoCsv))
	copy(buf, taginfoCsv)
	return buf
}

//go:embed taginfo.csv
var taginfoCsv []byte
