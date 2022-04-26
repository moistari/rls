package rls

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/moistari/rls/taginfo"
	"golang.org/x/text/transform"
)

func TestParseRelease(t *testing.T) {
	p := NewTagParser(taginfo.All(groupInfos()), DefaultLexers()...)
	m := make(map[string]bool)
	for n, tt := range rlsTests(t) {
		i, test := n, tt
		name := fmt.Sprintf("%s/%d", test.exp.Type, i)
		if strings.HasPrefix(name, "/") {
			name = "unknown" + name
		}
		t.Run(name, func(t *testing.T) {
			if _, ok := m[test.s]; ok {
				t.Fatalf("test %d %q is a duplicate!", i, test.s)
			}
			m[test.s] = true
			r := p.ParseRelease([]byte(test.s))
			if test.s != "" && r.Tags() == nil {
				t.Fatalf("test %d %q expected tags, got nil", i, test.s)
			}
			if s, exp := fmt.Sprintf("%o", r), test.s; s != exp {
				t.Errorf("test %d %q expected:\n  %q\ngot:\n  %q", i, test.s, exp, s)
			}
			var count int
			for _, tag := range r.Tags() {
				if tag.Is(TagTypeDate) {
					count++
				}
			}
			if count > 1 && r.Type != Magazine {
				t.Fatalf("test %d %q has TagTypeDate count > 1: %d", i, test.s, count)
			}
			v := buildRls(r)
			if !cmp.Equal(v, test.exp) {
				t.Errorf("test %d %q expected to be same, got:\n%s", i, test.s, cmp.Diff(test.exp, v))
			}
			// t.Logf("test %d\n  %q\n  %q", i, test.s, r.Title)
		})
	}
}

func TestCollapser(t *testing.T) {
	const test = "''\t\tAmélie\r\r1998\n\nMKV\f\f''"
	const exp = " Amelie 1998 MKV "
	a, _, err := transform.String(Clean, test)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if a != exp {
		t.Errorf("expected %q, got: %q", exp, a)
	}
	b, _, err := transform.String(Normalize, test)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if exp := strings.ToLower(exp); b != exp {
		t.Errorf("expected %q, got: %q", exp, b)
	}
}

func TestCompare(t *testing.T) {
	exp := []string{
		"",
		"1",
		"ii",
		"13",
		"xiii",
		"i",
		"i.am.legend",
		"'twas the night",
		"twas the night",
		"v",
		"v.for.vendetta",
		"a\tthing.1998.dvdrip",
		"a thing.1999.dvdrip",
		"Amélie.mkv",
		"amelie.1998.mkv",
		"Amélie.1999.mkv",
		"ghostbusters.mkv",
		"ghostbusters.ii.mkv",
		"ghostbusters.afterlife.mkv",
		"Harry.Potter.and.the.Sorcerer's.Stone.2001.Theatrical.Cut.mkv",
		"Harry.Potter.and.the.Chamber.of.Secrets.2002.Theatrical.Cut.mkv",
		"Harry.Potter.and.the.Prisoner.of.Azkaban.2004.mkv",
		"Harry.Potter.and.the.Goblet.of.Fire.2005.mkv",
		"Harry.Potter.and.the.Order.of.the.Phoenix.2007.mkv",
		"Harry.Potter.and.the.Half-Blood.Prince.2009.mkv",
		"Harry.Potter.and.the.Deathly.Hallows.Part.1.2010.mkv",
		"Harry.Potter.and.the.Deathly.Hallows.Part.2.2011.mkv",
		"i.am.legend.mkv",
		"rocky.mkv",
		"rocky ii.mkv",
		"rocky iii.mkv",
		"rocky iv.mkv",
		"rocky v.mkv",
		"rocky 6.mkv",
		"rocky 8.mkv",
		"rocky ix.mkv",
		"rocky x.mkv",
		"rocky 11.mkv",
		"the.matrix (part 2).1997.mkv",
		"The.Matrix.1999.mkv",
		"The.Matrix.Reloaded.2003.mkv",
		"The.Matrix.Revolutions.2004.mkv",
		"The.Matrix.Resurrections.2021.mkv",
		"The.Thomas.Crown.Affair.1968.720p.BluRay.AAC.2.0.x264-TDD.mkv",
		"The Thomas Crown Affair 1968 1080p BluRay AVC DTS-HD MA 2.0-CtrlHD",
		"The.Thomas.Crown.Affair.1968.4K.Remaster.720p.BluRay.AAC.2.0.x264-TDD.mkv",
		"The Thomas Crown Affair 1999 BluRay 1080p DTS-HD MA 5.1 AVC REMUX-FraMeSToR",
		"ultra vol. 1.mkv",
		"ultra vol 2.mkv",
		"ultra vol 3.1997.mkv",
		"ultra vol iii.1997.mkv",
		"ultra vol iv.mkv",
		"ultra vol. 8.mkv",
		"ultra vol ix.mkv",
		"ultra. vol. 13.mkv",
		"ultra vol xiii.mkv",
		"v.for.vendetta.mkv",
		"Zebra.S01E02",
		"Zébra.2009.S00.x264-group.mkv",
		"Zebra.2009.S01.FLAC-group",
		"Zebra.2009.S01E02",
		"Zébra.2009.S02",
		"the cc - A 1999.mp3",
		"the cc - a - the remix 1999.mp3",
		"minesweeper.winnt",
		"super.SMASH.brothers.nsw",
		"super.smash.brothers.nsw",
		"C.S. Lewis - Die Chroniken.von.Narnia - Der.Koenig.von.Narnia.Bd.II.2013.German.Retail.EPUB.eBook-BitBook",
		"C.S..Lewis.-.Die.Chroniken.von.Narnia ~ Der.Koenig.von.Narnia.Bd.1.2013.eBook-BitBook",
		"C.S..Lewis~Die.Chroniken.von.Narnia~Der.Koenig.von.Narnia.Bd.3.2013.German.Retail.EPUB.eBook-BitBook",
	}
	m := make(map[string]int)
	releases := make([]Release, len(exp))
	for i, s := range exp {
		if releases[i] = ParseString(s); s != "" && releases[i].Title == "" {
			t.Fatalf("test %d expected non-empty title for %q", i, s)
		}
		if _, ok := m[s]; ok {
			t.Fatalf("test %d %q is already defined in map", i, s)
		}
		m[s] = i
		// t.Logf("%d: %q: %s", i, s, releases[i].Type)
	}
	// randomize
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(releases), func(i, j int) {
		releases[i], releases[j] = releases[j], releases[i]
	})
	// sort
	sort.Slice(releases, func(i, j int) bool {
		return Compare(releases[i], releases[j]) < 0
	})
	// check
	v := make([]string, len(releases))
	for i := 0; i < len(releases); i++ {
		v[i] = fmt.Sprintf("%o", releases[i])
	}
	if !cmp.Equal(v, exp) {
		t.Errorf("expected to be same, got:\n%s", cmp.Diff(exp, v))
	}
}

func TestFind(t *testing.T) {
	f := genre()
	tags := []Tag{
		NewTag(TagTypeText, nil, []byte("a"), []byte("a")),
		NewTag(TagTypeText, nil, []byte("b"), []byte("b")),
		NewTag(TagTypeGenre, f, []byte("anime"), []byte("anime")),
		NewTag(TagTypeText, nil, []byte("a"), []byte("a")),
		NewTag(TagTypeGenre, f, []byte("horror"), []byte("horror")),
		NewTag(TagTypeText, nil, []byte("c"), []byte("c")),
	}
	for i, test := range []struct {
		f     string
		count int
		n     int
		m     string
		s     string
		types []TagType
	}{
		{"", -1, 6, "%o", "a b anime a horror c", nil},
		{"", -1, 6, "%s", "a b Anime a Horror c", nil},
		{"a", -1, 2, "%o", "a a", nil},
		{"a", -1, 2, "%s", "a a", nil},
		{"a", 1, 1, "%o", "a", nil},
		{"a", 1, 1, "%s", "a", nil},
		{"A", 1, 0, "%o", "", nil},
		{"A", 1, 0, "%s", "", nil},

		{"", -1, 4, "%s", "a b a c", []TagType{TagTypeText}}, // 8
		{"b", -1, 1, "%s", "b", []TagType{TagTypeText}},

		{"", -1, 2, "%o", "anime horror", []TagType{TagTypeGenre}}, // 10
		{"", -1, 2, "%s", "Anime Horror", []TagType{TagTypeGenre}},

		{"anime", -1, 1, "%o", "anime", nil}, // 12
		{"ANIME", -1, 0, "%o", "", nil},
		{"Anime", -1, 0, "%o", "", nil},
		{"anime", -1, 1, "%o", "anime", []TagType{TagTypeGenre}},
		{"ANIME", -1, 0, "%o", "", []TagType{TagTypeGenre}},
		{"Anime", -1, 0, "%o", "", []TagType{TagTypeGenre}},

		{"horror", -1, 1, "%s", "Horror", nil}, // 18
		{"HORROR", -1, 1, "%s", "Horror", nil},
		{"Horror", -1, 1, "%s", "Horror", nil},
		{"horror", -1, 1, "%s", "Horror", []TagType{TagTypeGenre}},
		{"HORROR", -1, 1, "%s", "Horror", []TagType{TagTypeGenre}},
		{"Horror", -1, 1, "%s", "Horror", []TagType{TagTypeGenre}},

		{"(?i)^anime$", -1, 1, "%r", "Anime", nil}, // 24
		{"(?i)^(anime|horror)$", -1, 2, "%r", "Anime Horror", nil},
	} {
		v, _ := Find(tags, test.f, test.count, rune(test.m[1]), test.types...)
		if n := len(v); n != test.n {
			t.Errorf("test %d expected %d, got: %d", i, test.n, n)
		}
		if s := joinTags(v, test.m, " "); s != test.s {
			t.Errorf("test %d expected %q, got: %q", i, test.s, s)
		}
	}
}

func TestTagInfo_find(t *testing.T) {
	f := genre()
	for i, test := range []string{"anime", "Anime", "ANIME", "ANiME"} {
		tag := NewTag(TagTypeGenre, f, []byte(test), []byte(test))
		info := tag.Info()
		if info == nil {
			t.Fatalf("test %d expected not nil", i)
		}
		if s, exp := info.Tag(), "Anime"; s != exp {
			t.Errorf("test %d expected %s, got: %s", i, exp, s)
		}
	}
}

func TestParseRoman(t *testing.T) {
	for i, test := range []struct {
		s   string
		exp int
		t   bool
	}{
		{"", 0, true},
		{"i", 1, true},
		{"ok", -1, false},
		{"more", -1, false},
		{"lcmxiv", -1, false}, // invalid 864
		{"dccclxiv", 864, true},
		{"cmxcix", 999, true},
		{"mm", 2000, true},
		{"mmiv", 2004, true},
		{"mmxvii", 2017, true},
		{"mmxviii", 2018, true},
	} {
		v, ok := parseRoman(test.s)
		if ok != test.t {
			t.Fatalf("test %d expected %t, got: %t", i, test.t, ok)
		}
		if v != test.exp {
			t.Errorf("test %d expected to be %d, got: %d", i, test.exp, v)
		}
		// t.Logf("%q: %d", test.s, v)
	}
}

func TestTagInfo_regexp(t *testing.T) {
	infos, err := taginfo.LoadAll()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	var types []string
	for k := range infos {
		types = append(types, k)
	}
	sort.Strings(types)
	for _, typ := range types {
		for i, info := range infos[typ] {
			tag := info.Tag()
			if strings.Contains(tag, "$") {
				t.Logf("skipping type %q line %d tag %s (%q)", typ, i+2, tag, info.Title())
				continue
			}
			restr := info.Regexp()
			if restr == "" {
				restr = tag
			}
			re, err := regexp.Compile(`^(?i)` + restr + `$`)
			if err != nil {
				t.Errorf("type %q line %d tag %s (%q) expected no error compiling %q, got: %v", typ, i+2, tag, info.Title(), info.Regexp(), err)
				continue
			}
			if !re.MatchString(info.Tag()) {
				t.Errorf("type %q line %d tag %s (%q) does not match %q", typ, i+2, tag, info.Title(), info.Regexp())
			}
		}
	}
}

func TestExport_tests(t *testing.T) {
	if s := os.Getenv("TESTS"); s != "export" {
		return
	}
	var keys []string
	m := make(map[string]rls)
	for i, test := range rlsTests(t) {
		if _, ok := m[test.s]; ok {
			t.Fatalf("test %d %q is a duplicate!", i, test.s)
		}
		keys, m[test.s] = append(keys, test.s), test.exp
	}
	sort.SliceStable(keys, func(i, j int) bool {
		var cmp int
		a, b := m[keys[i]], m[keys[j]]
		for _, f := range []func() int{
			compareInt(CompareMap[ParseType(a.Type)], CompareMap[ParseType(b.Type)]),
			compareTitle(a.Artist, b.Artist),
			compareTitle(a.Title, b.Title),
			compareInt(a.Year, b.Year),
			compareInt(a.Month, b.Month),
			compareInt(a.Day, b.Day),
			compareInt(a.Series, b.Series),
			compareInt(a.Episode, b.Episode),
			compareTitle(a.Subtitle, b.Subtitle),
			compareIntString(a.Resolution, b.Resolution),
			compareString(a.Version, b.Version),
			compareString(a.Group, b.Group),
			compareString(a.Title, b.Title),
			compareString(keys[i], keys[j]),
		} {
			if cmp = f(); cmp != 0 {
				break
			}
		}
		return cmp < 0
	})
	buf := new(bytes.Buffer)
	for i, key := range keys {
		fmt.Fprintf(buf, "%q: # %d\n", key, i)
		v := reflect.ValueOf(m[key])
		for j := 0; j < v.Type().NumField(); j++ {
			name := v.Type().Field(j).Name
			if name == "ID" {
				name = "id"
			}
			name = strings.ToLower(name[:1]) + name[1:]
			switch v.Field(j).Kind() {
			case reflect.Int:
				if i := v.Field(j).Int(); i != 0 {
					fmt.Fprintf(buf, "  %s: %d\n", name, i)
				}
			case reflect.String:
				if s := v.Field(j).String(); s != "" {
					fmt.Fprintf(buf, "  %s: %q\n", name, s)
				}
			default:
				t.Fatalf("unknown type %T", v.Field(i).Interface())
			}
		}
	}
	if err := ioutil.WriteFile("tests.yaml", buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestExport_taginfo(t *testing.T) {
	if s := os.Getenv("TESTS"); s != "export" {
		return
	}
	infos, err := taginfo.LoadFile("taginfo/taginfo.csv")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	var all [][]string
	for k, v := range infos {
		for _, info := range v {
			title := info.Title()
			if info.Tag() == title {
				title = ""
			}
			var excl string
			if info.Excl() {
				excl = "1"
			}
			all = append(all, []string{
				k,
				info.Tag(),
				title,
				info.Regexp(),
				info.Other(),
				Type(info.Type()).String(),
				excl,
			})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		var cmp int
		a, b := all[i], all[j]
		for _, f := range []func() int{
			compareString(a[0], b[0]),
			compareHasDollar(a[1], b[1]),
			compareResolution(a, b),
			compareRegion(a, b),
			comparePlatform(a, b),
			compareCodec(a, b),
			compareHDR(a, b),
			compareChannels(a, b),
			compareSuffix(a, b),
			comparePrefix(a[1], b[1]),
			compareInts(a[1], b[1]),
			compareString(strings.ToLower(a[1]), strings.ToLower(b[1])),
			compareString(a[1], b[1]),
		} {
			if cmp = f(); cmp != 0 {
				break
			}
		}
		return cmp < 0
	})
	buf := new(bytes.Buffer)
	_, _ = buf.WriteString("Type,Tag,Title,Regexp,Other,ReleaseType,TypeExclusive\n")
	for _, v := range all {
		_, _ = buf.WriteString(strings.Join(v, ",") + "\n")
	}
	if err := ioutil.WriteFile("taginfo/taginfo.csv", buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

func compareRegion(a, b []string) func() int {
	return func() int {
		if a[0] != "region" || b[0] != "region" {
			return 0
		}
		switch {
		case len(a[1]) < len(b[1]):
			return -1
		case len(b[1]) < len(a[1]):
			return 1
		}
		return strings.Compare(a[1], b[1])
	}
}

func comparePlatform(a, b []string) func() int {
	return func() int {
		as, bs := strings.ToLower(a[1]), strings.ToLower(b[1])
		if a[0] != "platform" || b[0] != "platform" || !strings.HasPrefix(as, "win") || !strings.HasPrefix(bs, "win") {
			return 0
		}
		switch {
		case len(a[1]) < len(b[1]):
			return 1
		case len(b[1]) < len(a[1]):
			return -1
		}
		return 0
	}
}

func compareResolution(a, b []string) func() int {
	return func() int {
		if a[0] != "resolution" || b[0] != "resolution" {
			return 0
		}
		switch {
		case a[1] == "PN.Selector" && b[1] != "PN.Selector":
			return -1
		case b[1] == "PN.Selector" && a[1] != "PN.Selector":
			return 1
		}
		return 0
	}
}

func compareCodec(a, b []string) func() int {
	return func() int {
		if a[0] != "codec" || b[0] != "codec" {
			return 0
		}
		switch {
		case len(a[1]) < len(b[1]):
			return 1
		case len(b[1]) < len(a[1]):
			return -1
		}
		return 0
	}
}

func compareHDR(a, b []string) func() int {
	return func() int {
		if a[0] != "hdr" || b[0] != "hdr" {
			return 0
		}
		switch {
		case len(a[1]) < len(b[1]):
			return 1
		case len(b[1]) < len(a[1]):
			return -1
		}
		return 0
	}
}

func compareChannels(a, b []string) func() int {
	return func() int {
		if a[0] != "channels" || b[0] != "channels" {
			return 0
		}
		switch cmp := strings.Compare(a[1], b[1]); {
		case cmp < 0:
			return 1
		case cmp > 0:
			return -1
		}
		return 0
	}
}

func comparePrefix(a, b string) func() int {
	return func() int {
		if a == b {
			return 0
		}
		switch {
		case strings.HasPrefix(strings.ToLower(a), strings.ToLower(b)):
			return -1
		case strings.HasPrefix(strings.ToLower(b), strings.ToLower(a)):
			return 1
		}
		return 0
	}
}

func compareSuffix(a, b []string) func() int {
	return func() int {
		if a[0] != "ext" || b[0] != "ext" || a[1] == b[1] {
			return 0
		}
		switch {
		case strings.HasSuffix(b[1], a[1]):
			return -1
		case strings.HasSuffix(a[1], b[1]):
			return 1
		}
		return 0
	}
}

func compareHasDollar(a, b string) func() int {
	return func() int {
		ac, bc := strings.Contains(a, "$"), strings.Contains(b, "$")
		switch {
		case ac && bc:
			return strings.Compare(a, b)
		case ac && !bc:
			return 1
		case bc && !ac:
			return -1
		}
		return 0
	}
}

func compareInts(a, b string) func() int {
	return func() int {
		am, bm := number.FindStringSubmatch(a), number.FindStringSubmatch(b)
		as, bs := number.ReplaceAllString(a, ""), number.ReplaceAllString(b, "")
		if len(am) != 0 && len(bm) != 0 && as != "" && bs != "" && as == bs {
			var cmp int
			switch cmp = compareIntString(a, b)(); {
			case cmp < 0:
				return 1
			case cmp > 0:
				return -1
			}
			return cmp
		}
		return strings.Compare(strings.ToLower(as), strings.ToLower(bs))
	}
}

var number = regexp.MustCompile(`\d+(?:\.\d+)?`)

type rls struct {
	Type string

	Artist   string
	Title    string
	Subtitle string

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

	Codec    string
	Hdr      string
	Audio    string
	Channels string
	Other    string
	Cut      string
	Edition  string
	Language string

	Size      string
	Region    string
	Container string
	Genre     string
	ID        string
	Group     string
	Meta      string
	Site      string
	Sum       string
	Pass      string
	Req       int
	Ext       string

	Unused string
}

func buildRls(r Release) rls {
	req := 0
	if r.Req {
		req = 1
	}
	return rls{
		Type: r.Type.String(),

		Artist:   r.Artist,
		Title:    r.Title,
		Subtitle: r.Subtitle,

		Platform: r.Platform,
		Arch:     r.Arch,

		Source:     r.Source,
		Resolution: r.Resolution,
		Collection: r.Collection,

		Year:  r.Year,
		Month: r.Month,
		Day:   r.Day,

		Series:  r.Series,
		Episode: r.Episode,
		Version: r.Version,
		Disc:    r.Disc,

		Codec:    strings.Join(r.Codec, " "),
		Hdr:      strings.Join(r.Hdr, " "),
		Audio:    strings.Join(r.Audio, " "),
		Channels: r.Channels,
		Other:    strings.Join(r.Other, " "),
		Cut:      strings.Join(r.Cut, " "),
		Edition:  strings.Join(r.Edition, " "),
		Language: strings.Join(r.Language, " "),

		Size:      r.Size,
		Region:    r.Region,
		Container: r.Container,
		Genre:     r.Genre,
		ID:        r.ID,
		Group:     r.Group,
		Meta:      strings.Join(r.Meta, " "),
		Site:      r.Site,
		Sum:       r.Sum,
		Pass:      r.Pass,
		Ext:       r.Ext,
		Req:       req,
		Unused:    joinTags(r.Unused(), "%s", " "),
	}
}

type rlsTest struct {
	s   string
	exp rls
}

func rlsTests(tb testing.TB) []rlsTest {
	buf, err := ioutil.ReadFile("tests.yaml")
	if err != nil {
		tb.Fatal(err)
	}
	s := bufio.NewScanner(bytes.NewReader(buf))
	var tests []rlsTest
	test := rlsTest{}
	var count int
	for i, n := 0, 0; s.Scan(); count, i = count+1, i+1 {
		switch line := s.Bytes(); {
		case bytes.HasPrefix(line, []byte(`"`)):
			if i != 0 {
				tests, test = append(tests, test), rlsTest{}
			}
			if n = bytes.LastIndexByte(line, '"'); n == -1 {
				tb.Fatalf("unable to locate \" on line %d item %d: %q", count, i, string(line))
			}
			var err error
			test.s, err = strconv.Unquote(string(line[:n+1]))
			if err != nil {
				tb.Fatalf("unable to unquote line %d item %d: %v", count, i, err)
			}
		case bytes.HasPrefix(line, []byte(`  `)):
			if n = bytes.IndexByte(line, ':'); n == -1 {
				tb.Fatalf("unable to locate : on line %d item %d", count, i)
			}
			name := strings.ToUpper(string(line[2:3])) + string(line[3:n])
			if name == "Id" {
				name = "ID"
			}
			f := reflect.ValueOf(&test.exp).Elem().FieldByName(name)
			switch f.Kind() {
			case reflect.Int:
				i, err := strconv.ParseInt(string(line[n+2:]), 10, 64)
				if err != nil {
					tb.Fatalf("unable to convert int for %s on line %d: %v", name, count, err)
				}
				f.SetInt(i)
			case reflect.String:
				s, err := strconv.Unquote(string(line[n+2:]))
				if err != nil {
					tb.Fatalf("unable to unquote string for %s on line %d: %v", name, count, err)
				}
				f.SetString(s)
			}
		default:
			tb.Fatalf("unknown line %d item %d: %q", count, i, string(line))
		}
	}
	if err := s.Err(); err != nil {
		tb.Fatal(err)
	}
	return append(tests, test)
}

// genre returns a find func for the embedded genres.
func genre() taginfo.FindFunc {
	return taginfo.Find(taginfo.All()["genre"]...)
}

// joinTags joins tags using a formatting string and a separator.
func joinTags(tags []Tag, str, sep string) string {
	var v []string
	for _, tag := range tags {
		v = append(v, fmt.Sprintf(str, tag))
	}
	return strings.Join(v, sep)
}

func groupInfos() map[string][]*taginfo.Taginfo {
	var groups []*taginfo.Taginfo
	for _, group := range []struct {
		tag, typ string
	}{
		{"CODEX", "game"},
		{"DARKSiDERS", "game"},
		{"D-Z0N3", "movie"},
		{"MrSeeN-SiMPLE", ""},
	} {
		groups = append(groups, taginfo.Must(group.tag, "", "", "", group.typ, ""))
	}
	return map[string][]*taginfo.Taginfo{
		"group": groups,
	}
}
