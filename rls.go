// Package rls parses release information.
package rls

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/moistari/rls/taginfo"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

//go:generate stringer -type TagType -trimprefix TagType

// Release is release information.
type Release struct {
	Type Type

	Artist   string
	Title    string
	Subtitle string
	Alt      string

	Platform string
	Arch     string

	Source     string
	Resolution string
	Collection string

	Year  int
	Month int
	Day   int

	Series  int
	Episode int
	Version string
	Disc    string

	Codec    []string
	HDR      []string
	Audio    []string
	Channels string
	Other    []string
	Cut      []string
	Edition  []string
	Language []string

	Size      string
	Region    string
	Container string
	Genre     string
	ID        string
	Group     string
	Meta      []string
	Site      string
	Sum       string
	Pass      string
	Req       bool
	Ext       string

	tags   []Tag
	dates  []int
	unused []int
	end    int
}

// Parse creates a release from src.
func Parse(src []byte) Release {
	return DefaultParser.ParseRelease(src)
}

// ParseString creates a release from src
func ParseString(src string) Release {
	return DefaultParser.ParseRelease([]byte(src))
}

// Format satisfies the fmt.Formatter interface.
//
// Format Options:
//
//	o - original release
//	v - tag type followed by colon and quoted capture value (Date:["2009", "", ""])
//	s - normalized capture value (2009)
//	e - tag type and normal value (as in s) surrounded by angle brackets (<Date:2009>)
//	q - original captured value, quoted
func (r Release) Format(f fmt.State, verb rune) {
	switch verb {
	case 'q':
		buf := new(bytes.Buffer)
		for _, tag := range r.tags {
			if _, err := fmt.Fprintf(buf, "%o", tag); err != nil {
				panic(err)
			}
		}
		fmt.Fprintf(f, "%q", buf.Bytes())
	case 's', 'o', 'e':
		for _, tag := range r.tags {
			tag.Format(f, verb)
		}
	}
}

// String satisfies the fmt.Stringer interface.
func (r Release) String() string {
	var v []string
	for _, tag := range r.tags {
		v = append(v, fmt.Sprintf("%o", tag))
	}
	return strings.Join(v, "")
}

// Tags returns all tags.
func (r Release) Tags() []Tag {
	return r.tags
}

// Unused returns text tags not used in titles.
func (r Release) Unused() []Tag {
	var unused []Tag
	for _, i := range r.unused {
		unused = append(unused, r.tags[i])
	}
	return unused
}

// Dates returns date tags not used.
func (r Release) Dates() []Tag {
	var dates []Tag
	for _, i := range r.dates {
		dates = append(dates, r.tags[i])
	}
	return dates
}

// Tag is a release tag.
type Tag struct {
	typ   TagType
	v     []string
	f     taginfo.FindFunc
	prev  TagType
	prevf taginfo.FindFunc
}

// NewTag creates a new tag.
func NewTag(typ TagType, f taginfo.FindFunc, b ...[]byte) Tag {
	if len(b) < 2 {
		panic("must provide at least 2 values to NewTag")
	}
	v := make([]string, len(b))
	for i := 0; i < len(b); i++ {
		v[i] = string(b[i])
	}
	return Tag{
		typ: typ,
		v:   v,
		f:   f,
	}
}

// ParseTags parses tags from src.
func ParseTags(src []byte) ([]Tag, int) {
	return DefaultParser.Parse(src)
}

// ParseTagsString parses tags from src.
func ParseTagsString(src string) ([]Tag, int) {
	return DefaultParser.Parse([]byte(src))
}

// As returns a copy of tag as a tag of the specified type.
func (tag Tag) As(typ TagType, f taginfo.FindFunc) Tag {
	return Tag{
		typ:   typ,
		f:     f,
		v:     tag.v,
		prev:  tag.typ,
		prevf: tag.f,
	}
}

// Revert returns a copy of tag as the tag's previous type.
func (tag Tag) Revert() Tag {
	return Tag{
		typ: tag.prev,
		f:   tag.prevf,
		v:   tag.v,
	}
}

// Is returns true when tag is of a type.
func (tag Tag) Is(types ...TagType) bool {
	for _, typ := range types {
		if tag.typ == typ {
			return true
		}
	}
	return false
}

// Was returns true when tag's prev is of a type.
func (tag Tag) Was(types ...TagType) bool {
	for _, typ := range types {
		if tag.prev == typ {
			return true
		}
	}
	return false
}

// Info retrieves the tag's tag info.
func (tag Tag) Info() *taginfo.Taginfo {
	if tag.f != nil {
		return tag.f(tag.Normalize())
	}
	return nil
}

// SingleEp returns true when the
func (tag Tag) SingleEp() bool {
	if tag.typ.Is(TagTypeSeries) {
		s, e := tag.Series()
		return s == 0 && e != 0 && tag.v[1] == "" && tag.v[0] == tag.v[2]
	}
	return false
}

// InfoType returns the associated tag info type.
func (tag Tag) InfoType() Type {
	if info := tag.Info(); info != nil {
		return Type(info.Type())
	}
	return Unknown
}

// InfoExcl returns the associated tag info excl.
func (tag Tag) InfoExcl() bool {
	if info := tag.Info(); info != nil {
		return info.Excl()
	}
	return false
}

// TagType returns the tag's tag type.
func (tag Tag) TagType() TagType {
	return tag.typ
}

// Prev returns the tag's previous tag type.
func (tag Tag) Prev() TagType {
	return tag.prev
}

// InfoTitle retrieves the tag's title.
func (tag Tag) InfoTitle() string {
	if info := tag.Info(); info != nil {
		s := info.Title()
		for i := 1; i < len(tag.v); i++ {
			s = strings.ReplaceAll(s, "$"+strconv.Itoa(i+1), string(tag.v[i]))
		}
		return s
	}
	return ""
}

// normalize attempts to normalize s, replacing $1, $2, ... $N,  with the
// values in v.
func (tag Tag) normalize(s string, v ...string) string {
	if tag.f != nil {
		if info := tag.f(s); info != nil {
			s = info.Tag()
		}
		for i := 0; i < len(v); i++ {
			s = strings.ReplaceAll(s, "$"+strconv.Itoa(i+1), v[i])
		}
	}
	return s
}

// Normalize returns the normalized string for the tag.
func (tag Tag) Normalize() string {
	switch tag.typ {
	case TagTypeWhitespace:
		return tag.Whitespace()
	case TagTypeDelim:
		return tag.Delim()
	case TagTypeText:
		return tag.Text()
	case TagTypePlatform:
		return tag.Platform()
	case TagTypeArch:
		return tag.Arch()
	case TagTypeSource:
		return tag.Source()
	case TagTypeResolution:
		return tag.Resolution()
	case TagTypeCollection:
		return tag.Collection()
	case TagTypeDate:
		year, month, day := tag.Date()
		if month != 0 && day != 0 {
			return fmt.Sprintf("%d-%02d-%02d", year, month, day)
		}
		return strconv.Itoa(year)
	case TagTypeSeries:
		series, episode := tag.Series()
		if episode != 0 {
			return fmt.Sprintf("S%02dE%02d", series, episode)
		}
		return fmt.Sprintf("S%02d", series)
	case TagTypeVersion:
		return tag.Version()
	case TagTypeDisc:
		return tag.Disc()
	case TagTypeCodec:
		return tag.Codec()
	case TagTypeHDR:
		return tag.HDR()
	case TagTypeAudio:
		return tag.Audio()
	case TagTypeChannels:
		return tag.Channels()
	case TagTypeOther:
		return tag.Other()
	case TagTypeCut:
		return tag.Cut()
	case TagTypeEdition:
		return tag.Edition()
	case TagTypeLanguage:
		return tag.Language()
	case TagTypeSize:
		return tag.Size()
	case TagTypeRegion:
		return tag.Region()
	case TagTypeContainer:
		return tag.Container()
	case TagTypeGenre:
		return tag.Genre()
	case TagTypeID:
		return tag.ID()
	case TagTypeGroup:
		return tag.Group()
	case TagTypeMeta:
		typ, s := tag.Meta()
		switch typ {
		case "site", "sum":
			return "[" + s + "]"
		case "pass":
			return "{{" + s + "}}"
		case "req":
			return "[REQ]"
		}
		return "[[" + typ + ":" + s + "]]"
	case TagTypeExt:
		return tag.Ext()
	}
	return ""
}

// Match determines if s matches the tag.
func (tag Tag) Match(s string, verb rune, types ...TagType) bool {
	if len(types) != 0 && !tag.Is(types...) {
		return false
	}
	v := fmt.Sprintf("%"+string(verb), tag)
	switch {
	case s == "":
		return true
	case tag.f != nil && (verb == 's'):
		if info := tag.f(s); info != nil {
			s = info.Tag()
		}
	}
	if verb == 'r' {
		return regexp.MustCompile(s).MatchString(v)
	}
	return s == v
}

// Format satisfies the fmt.Formatter interface.
//
// Format Options:
//
//	q - all values including captured values, quoted (["2009", "2009", "", ""])
//	o - original capture (2009)
//	v - tag type followed by colon and quoted capture value (Date:["2009", "", ""])
//	s - normalized capture value (2009)
//	r - same as s
//	e - tag type and normal value (as in s) surrounded by angle brackets (<Date:2009>)
func (tag Tag) Format(f fmt.State, verb rune) {
	var buf []byte
	switch verb {
	case 'q':
		buf = append(buf, fmt.Sprintf("%q", tag.v)...)
	case 'o':
		buf = append(buf, tag.v[0]...)
	case 'v':
		buf = append(buf, fmt.Sprintf("%s:%q", tag.typ, tag.v[1:])...)
	case 'e':
		s := strconv.Quote(tag.Normalize())
		buf = append(buf, "<"+tag.typ.String()+":"+s[1:len(s)-1]+">"...)
	case 's', 'r':
		buf = append(buf, tag.Normalize()...)
	}
	_, _ = f.Write(buf)
}

// Whitespace normalizes the whitespace value.
func (tag Tag) Whitespace() string {
	return tag.v[1]
}

// Delim normalizes the delimiter value.
func (tag Tag) Delim() string {
	return tag.v[1]
}

// Text normalizes the text value.
func (tag Tag) Text() string {
	switch tag.prev {
	case TagTypeDate, TagTypeSeries:
		return tag.v[0]
	case TagTypeChannels:
		return tag.Channels()
	}
	return tag.v[1]
}

// TextReplace normalizes the text value and replaces the supplied string.
func (tag Tag) TextReplace(old, newstr string, n int) string {
	s := tag.Text()
	if tag.prev == TagTypeChannels {
		return s
	}
	return strings.Replace(s, old, newstr, n)
}

// Platform normalizes the platform value.
func (tag Tag) Platform() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// Arch normalizes the arch value.
func (tag Tag) Arch() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// Source normalizes the source value.
func (tag Tag) Source() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// Resolution normalizes the resolution value.
func (tag Tag) Resolution() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// Collection normalizes the collection value.
func (tag Tag) Collection() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// Date normalizes the date value.
func (tag Tag) Date() (int, int, int) {
	year, _ := strconv.Atoi(tag.v[1])
	month, _ := strconv.Atoi(tag.v[2])
	day, _ := strconv.Atoi(tag.v[3])
	return year, month, day
}

// Series normalizes the series value.
func (tag Tag) Series() (int, int) {
	series, _ := strconv.Atoi(tag.v[1])
	episode, _ := strconv.Atoi(tag.v[2])
	return series, episode
}

// Version normalizes the version value.
func (tag Tag) Version() string {
	return tag.v[1]
}

// Disc normmalizes the disc value.
func (tag Tag) Disc() string {
	disc, _ := strconv.Atoi(tag.v[2])
	switch tag.v[1] {
	case "CD", "DVD":
		return fmt.Sprintf("%s%d", tag.v[1], disc)
	case "S":
		return fmt.Sprintf("%dDiSCS", disc)
	case "X":
		return fmt.Sprintf("%dx", disc)
	}
	return fmt.Sprintf("D%02d", disc)
}

// Codec normalizes a codec value.
func (tag Tag) Codec() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// HDR normalizes a hdr value.
func (tag Tag) HDR() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// Audio normalizes an audio value.
func (tag Tag) Audio() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// Channels normalizes an channels value.
func (tag Tag) Channels() string {
	s := strings.Map(func(r rune) rune {
		switch r {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return r
		}
		return -1
	}, tag.normalize(tag.v[1]))
	return s[:1] + "." + s[1:]
}

// Other normalizes the other value.
func (tag Tag) Other() string {
	s := tag.normalize(tag.v[1], tag.v[2:]...)
	switch y := strings.ToUpper(s); y {
	case "19XX", "20XX":
		return y
	}
	return s
}

// Cut normalizes the cut value.
func (tag Tag) Cut() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// Edition normalizes the edition value.
func (tag Tag) Edition() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// Language normalizes the language value.
func (tag Tag) Language() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// Size normalizes the size value.
func (tag Tag) Size() string {
	return strings.ReplaceAll(strings.ToUpper(tag.normalize(tag.v[1], tag.v[2:]...)), "I", "i")
}

// Region normalizes the region value.
func (tag Tag) Region() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// Container normalizes the container value.
func (tag Tag) Container() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// Genre normalizes the genre value.
func (tag Tag) Genre() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// ID normalizes the id value.
func (tag Tag) ID() string {
	return tag.normalize(tag.v[1], tag.v[2:]...)
}

// Group normalizes the group value.
func (tag Tag) Group() string {
	return tag.v[1]
}

// Meta normalizes a file meta value.
func (tag Tag) Meta() (string, string) {
	return tag.v[1], tag.v[2]
}

// Ext normalizes a file ext value.
func (tag Tag) Ext() string {
	return strings.ToLower(tag.v[1])
}

// TagType is a tag type.
type TagType int

// TagType values.
const (
	TagTypeWhitespace TagType = iota
	TagTypeDelim
	TagTypeText
	TagTypePlatform
	TagTypeArch
	TagTypeSource
	TagTypeResolution
	TagTypeCollection
	TagTypeDate
	TagTypeSeries
	TagTypeVersion
	TagTypeDisc
	TagTypeCodec
	TagTypeHDR
	TagTypeAudio
	TagTypeChannels
	TagTypeOther
	TagTypeCut
	TagTypeEdition
	TagTypeLanguage
	TagTypeSize
	TagTypeRegion
	TagTypeContainer
	TagTypeGenre
	TagTypeID
	TagTypeGroup
	TagTypeMeta
	TagTypeExt
)

// Is returns true when tag type is in types.
func (typ TagType) Is(types ...TagType) bool {
	for _, t := range types {
		if typ == t {
			return true
		}
	}
	return false
}

// Type is a release type.
type Type int

// Release types.
const (
	Unknown Type = iota
	App
	Audiobook
	Book
	Comic
	Education
	Episode
	Game
	Magazine
	Movie
	Music
	Series
)

// ParseType parses a type from s.
func ParseType(s string) Type {
	switch s {
	case "app":
		return App
	case "audiobook":
		return Audiobook
	case "book":
		return Book
	case "comic":
		return Comic
	case "education":
		return Education
	case "episode":
		return Episode
	case "game":
		return Game
	case "magazine":
		return Magazine
	case "movie":
		return Movie
	case "music":
		return Music
	case "series":
		return Series
	}
	return Unknown
}

// String satisfies the fmt.Stringer interface.
func (typ Type) String() string {
	switch typ {
	case App:
		return "app"
	case Audiobook:
		return "audiobook"
	case Book:
		return "book"
	case Comic:
		return "comic"
	case Education:
		return "education"
	case Episode:
		return "episode"
	case Game:
		return "game"
	case Magazine:
		return "magazine"
	case Movie:
		return "movie"
	case Music:
		return "music"
	case Series:
		return "series"
	}
	return ""
}

// Is returns true when the type is in types.
func (typ Type) Is(types ...Type) bool {
	for _, t := range types {
		if typ == t {
			return true
		}
	}
	return false
}

// Builder is the interface for release builders.
type Builder interface {
	Build([]Tag, int) Release
}

// Parser is the interface for parsers.
type Parser interface {
	Parse([]byte) ([]Tag, int)
	ParseRelease([]byte) Release
}

// NewDefaultParser creates a new default tag parser.
func NewDefaultParser() Parser {
	return NewTagParser(taginfo.All(), DefaultLexers()...)
}

// DefaultBuilder is the default release tag builder.
var DefaultBuilder Builder

// DefaultParser is the default tag parser.
var DefaultParser Parser

func init() {
	for i := Unknown; i <= Series; i++ {
		taginfo.RegisterType(i.String(), int(i))
	}
	DefaultBuilder = NewTagBuilder()
	DefaultParser = NewDefaultParser()
}

// CompareMap is the release compare map. Modifying this will alter the order
// by which different release types are grouped together when using Compare
// with a sort operation.
var CompareMap = map[Type]int{
	Unknown:   0,
	Movie:     1,
	Series:    2,
	Episode:   2,
	Music:     3,
	App:       4,
	Game:      5,
	Book:      6,
	Audiobook: 7,
	Comic:     9,
	Education: 8,
	Magazine:  10,
}

// Compare compares a to b, normalizing titles with Normalize, comparing the
// resulting lower cased strings. Release types are grouped together based on
// the precedence defined in CompareMap.
func Compare(a, b Release) int {
	var cmp int
	for _, f := range []func() int{
		compareInt(CompareMap[a.Type], CompareMap[b.Type]),
		compareTitle(a.Artist, b.Artist),
		compareTitle(a.Title, b.Title),
		compareInt(a.Year, b.Year),
		compareInt(a.Month, b.Month),
		compareInt(a.Day, b.Day),
		compareInt(a.Series, b.Series),
		compareInt(a.Episode, b.Episode),
		compareTitle(a.Subtitle, b.Subtitle),
		compareTitle(a.Alt, b.Alt),
		compareIntString(a.Resolution, b.Resolution),
		compareString(a.Version, b.Version),
		compareString(a.Group, b.Group),
		compareString(a.Title, b.Title),
		compareOriginal(a, b),
	} {
		if cmp = f(); cmp != 0 {
			return cmp
		}
	}
	return cmp
}

// compareInt returns a func that compares a, b.
func compareInt(a, b int) func() int {
	return func() int {
		switch {
		case a < b:
			return -1
		case b < a:
			return 1
		}
		return 0
	}
}

// compareIntString returns a func that compares numbers in a, b.
func compareIntString(a, b string) func() int {
	return func() int {
		switch {
		case a == b:
			return 0
		case a == "" && b != "":
			return -1
		case b == "" && a != "":
			return 1
		case !strings.ContainsAny(a, "0123456789"):
			return 0
		case !strings.ContainsAny(b, "0123456789"):
			return 1
		}
		var ai, bi int
		for i := len(a); i > 0; i-- {
			if f := float64(a[i-1] - '0'); f >= 0 {
				ai += int(f * math.Pow(10, float64(len(a)-i)))
			}
		}
		for i := len(b); i > 0; i-- {
			if f := float64(b[i-1] - '0'); f >= 0 {
				bi += int(f * math.Pow(10, float64(len(b)-i)))
			}
		}
		switch {
		case ai < bi:
			return -1
		case bi < ai:
			return 1
		}
		return 0
	}
}

// compareTitle returns a func that does a title comparison of a, b.
func compareTitle(a, b string) func() int {
	// const cutset = "\t\n\f\r -._,()[]{}+\\/~"
	return func() int {
		switch {
		case a == b:
			return 0
		case a == "" && b != "":
			return -1
		case b == "" && a != "":
			return 1
		}
		a, b := MustNormalize(a), MustNormalize(b)
		av, bv := strings.FieldsFunc(strings.ToLower(a), isBreakDelim), strings.FieldsFunc(strings.ToLower(b), isBreakDelim)
		start, min := 0, 3
		if len(av) > 0 && len(bv) > 0 && av[0] == bv[0] && contains([]string{"a", "an", "the"}, av[0]) {
			start, min = 1, 1
		}
		// compare fields up to a prefix length
		i := start
		for ; i < start+min && i < len(av) && i < len(bv); i++ {
			switch {
			case av[i] == "and" && bv[i] == "&", av[i] == "&" && bv[i] == "and":
				continue
			}
			if cmp := compareTitleNumber(av[i], bv[i], i); cmp != 0 {
				return cmp
			}
		}
		if i < start+min {
			// haven't reached end of prefixes and a or b is prefix of the
			// other, determine which is shorter
			switch {
			case len(av) < len(bv):
				return -1
			case len(bv) < len(av):
				return +1
			}
		}
		return 0
	}
}

// compareTitleNumber compares a, b as numbers if both are numbers or roman
// numerals (such as VI or 6).
func compareTitleNumber(a, b string, i int) int {
	ai, arom, aok := convNumber(a)
	bi, brom, bok := convNumber(b)
	abad := i == 0 && arom && aok && (ai == 1 || ai == 5 || ai == 50)
	bbad := i == 0 && brom && bok && (bi == 1 || bi == 5 || bi == 50)
	switch {
	case (arom || brom) && i == 0:
	case abad && bbad:
	case aok && bbad:
		return -1
	case bok && abad:
		return +1
	case aok && bok && ai < bi, aok && !bok && !abad:
		return -1
	case aok && bok && bi < ai, bok && !aok && !bbad:
		return +1
	}
	return strings.Compare(a, b)
}

// compareString returns a func that compares a, b.
func compareString(a, b string) func() int {
	return func() int {
		return strings.Compare(a, b)
	}
}

// compareOriginal returns a func that compares a, b's original string.
func compareOriginal(a, b Release) func() int {
	return func() int {
		return strings.Compare(fmt.Sprintf("%o", a), fmt.Sprintf("%o", b))
	}
}

// isAnyDelim returns true if r is any delimiter.
func isAnyDelim(r rune) bool {
	switch r {
	case '\t', '\n', '\f', '\r', ' ', '(', ')', '+', ',', '-', '.', '_', '[', '/', '\\', ']', '{', '}', '~':
		return true
	}
	return false
}

// isBreakDelim returns true if r is a break delimiter (same as any, excluding
// '-').
func isBreakDelim(r rune) bool {
	switch r {
	case '\t', '\n', '\f', '\r', ' ', '(', ')', '+', ',' /*, '-'*/, '.', '_', '[', '/', '\\', ']', '{', '}', '~':
		return true
	}
	return false
}

// isTitleTrimDelim returns true if r is a title trim delimiter (any delim
// except '.', '+').
func isTitleTrimDelim(r rune) bool {
	switch r {
	case '\t', '\n', '\f', '\r', ' ', '(', ')' /*, '+'*/, ',', '-' /*, '.'*/, '_', '[', '/', '\\', ']', '{', '}', '~':
		return true
	}
	return false
}

// convNumber attempts to convert a int or roman numeral.
func convNumber(s string) (int, bool, bool) {
	if i, err := strconv.Atoi(s); err == nil {
		return i, false, true
	} else if i, ok := parseRoman(s); ok && i < 100 {
		return i, true, true
	}
	return 0, false, false
}

// parseRoman parses roman numerals.
func parseRoman(s string) (int, bool) {
	if s == "" {
		return 0, true
	}
	var i, r int
	for j := 0; j < len(s); j++ {
		switch r = roman(s[j]); {
		case r == 0, j < len(s)-2 && r < roman(s[j+1]) && roman(s[j+1]) < roman(s[j+2]):
			return -1, false
		case j < len(s)-1 && r < roman(s[j+1]):
			i -= r
		default:
			i += r
		}
	}
	return i, true
}

// roman returns the value for a roman numeral.
func roman(c byte) int {
	switch c {
	case 'i':
		return 1
	case 'v':
		return 5
	case 'x':
		return 10
	case 'l':
		return 50
	case 'c':
		return 100
	case 'd':
		return 500
	case 'm':
		return 1000
	}
	return 0
}

// Find finds a tag.
func Find(tags []Tag, s string, count int, verb rune, types ...TagType) ([]Tag, int) {
	if count == -1 {
		count = len(tags)
	}
	// copy
	if s == "" && len(types) == 0 {
		v := make([]Tag, count)
		copy(v, tags[:count])
		return v, count
	}
	var v []Tag
	var i int
	// collect matching
	for ; i < len(tags) && len(v) < count; i++ {
		if tags[i].Match(s, verb, types...) {
			v = append(v, tags[i])
		}
	}
	return v, i
}

// NewCleaner creates a text transformer chain that transforms text to its
// textual decomposed clean form (NFD), removing all non-spacing marks,
// converting all spaces to ' ', removing ', collapsing adjacent spaces into a
// single ' ', and finally returning the canonical normalized form (NFC).
//
// See: https://go.dev/blog/normalization
func NewCleaner() transform.Transformer {
	return transform.Chain(
		norm.NFD,
		NewCollapser(
			false, true,
			`'`, " \t\r\n\f",
			nil,
		),
		norm.NFC,
	)
}

// MustClean applies the Clean transform to s.
func MustClean(s string) string {
	s, _, err := transform.String(NewCleaner(), s)
	if err != nil {
		panic(err)
	}
	return s
}

// NewNormalizer creates a new a text transformer chain (similiar to
// NewCleaner) that normalizes text to lower case clean form useful for
// matching titles.
//
// See: https://go.dev/blog/normalization
func NewNormalizer() transform.Transformer {
	return transform.Chain(
		norm.NFD,
		NewCollapser(
			true, true,
			"`"+`':;~!@#%^*=+()[]{}<>/?|\",`, " \t\r\n\f._",
			func(r, prev, next rune) rune {
				switch {
				case r == '-' && unicode.IsSpace(prev):
					return -1
				case r == '$' && (unicode.IsLetter(prev) || unicode.IsLetter(next)):
					return 'S'
				case r == '£' && (unicode.IsLetter(prev) || unicode.IsLetter(next)):
					return 'L'
				case r == '$', r == '£':
					return -1
				}
				return r
			},
		),
		norm.NFC,
	)
}

// MustNormalize applies the Normalize transform to s, returning a lower cased,
// clean form of s useful for matching titles.
func MustNormalize(s string) string {
	s, _, err := transform.String(NewNormalizer(), s)
	if err != nil {
		panic(err)
	}
	return s
}

// Collapser is a transform.Transformer that converts spaces to ' ', removes
// specified runes, and collapses adjacent spaces to a single ' '.
//
// See: https://go.dev/blog/normalization
type Collapser struct {
	// Spc is the space rune.
	Spc rune
	// Lower toggles lower casing.
	Lower bool
	// Trim toggles trimming leading/trailing spaces.
	Trim bool
	// Remove are the runes to remove.
	Remove map[rune]bool
	// Space are the runes to convert to Spc.
	Space map[rune]bool
	// Transformer is a rune transformer.
	Transformer func(rune, rune, rune) rune
}

// NewCollapser creates a text transformer that collapses spaces, removes
// specific runes, optionally lower cases runes, and removes non-spacing marks.
//
// Ideally used between a transform chain of norm.NFD and norm.NFC.
//
// Clean, Normalize, and MustClean, MustNormalize are provided for utility
// purposes.
//
// See: https://go.dev/blog/normalization
func NewCollapser(lower, trim bool, remove, space string, transformer func(rune, rune, rune) rune) Collapser {
	rem, spc := make(map[rune]bool), make(map[rune]bool)
	for _, r := range remove {
		rem[r] = true
	}
	for _, r := range space {
		spc[r] = true
	}
	return Collapser{
		Spc:         ' ',
		Lower:       lower,
		Trim:        trim,
		Remove:      rem,
		Space:       spc,
		Transformer: transformer,
	}
}

// Transform satisfies the transform.Transformer interface.
func (c Collapser) Transform(dst, src []byte, atEOF bool) (int, int, error) {
	var i, l, j, n int
	var prev, r rune
	b, s, d := make([]byte, utf8.UTFMax), len(src), len(dst)
	// trim leading spaces
	if c.Trim {
		for ; i < s; i = i + l {
			if r, l = utf8.DecodeRune(src[i:]); r == utf8.RuneError {
				return n, i + l, transform.ErrShortSrc
			}
			if !c.Remove[r] && !c.Space[r] {
				break
			}
		}
	}
	// copy from src to dst
	var last int
	for ; i < s; i = i + l {
		r, l = utf8.DecodeRune(src[i:])
		switch {
		case r == utf8.RuneError:
			return n, i + l, transform.ErrShortSrc
		case c.Space[r] && prev == c.Spc, c.Remove[r], unicode.Is(unicode.Mn, r):
			continue
		case c.Space[r]:
			r = c.Spc
		}
		if c.Transformer != nil {
			if r = c.Transformer(r, prev, rpeek(src, i+l, s)); r == -1 {
				continue
			}
		}
		if c.Lower {
			r = unicode.ToLower(r)
		}
		if j = utf8.EncodeRune(b, r); d < n+j {
			return n, i, transform.ErrShortDst
		}
		copy(dst[n:], b[:j])
		prev, n = r, n+j
		if r != c.Spc {
			last = n
		}
	}
	if c.Trim {
		return last, i, nil
	}
	return n, i, nil
}

// Reset satisfies the transform.Transformer interface.
func (Collapser) Reset() {}

// rpeek
func rpeek(b []byte, i, n int) rune {
	if i < n {
		if r, _ := utf8.DecodeRune(b[i:]); r != utf8.RuneError {
			return r
		}
	}
	return 0
}
