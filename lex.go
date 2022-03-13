package rls

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/moistari/rls/reutil"
	"github.com/moistari/rls/taginfo"
)

// LexFunc is the signature for lexer funcs.
type LexFunc func([]byte, []byte, []Tag, []Tag, int, int) ([]Tag, []Tag, int, int, bool)

// Lexer is the interface for lexers.
type Lexer interface {
	Initialize(map[string][]*taginfo.Taginfo, *regexp.Regexp, map[string]bool) (LexFunc, bool)
}

// TagLexer is a tag lexer.
type TagLexer struct {
	Init func(map[string][]*taginfo.Taginfo, *regexp.Regexp, map[string]bool)
	Lex  LexFunc
	Once bool
}

// Init satisfies the Lexer interface.
func (lexer TagLexer) Initialize(infos map[string][]*taginfo.Taginfo, delim *regexp.Regexp, short map[string]bool) (LexFunc, bool) {
	if lexer.Init != nil {
		lexer.Init(infos, delim, short)
	}
	return lexer.Lex, lexer.Once
}

// DefaultLexers returns the default tag tag lexers.
func DefaultLexers() []Lexer {
	return []Lexer{
		// --------------- once ---------------
		NewTrimWhitespaceLexer(),
		NewExtLexer(),
		NewMetaLexer(
			// [[ type:value ]]
			``, `[[`, `]]`, `([a-zA-Z][a-zA-Z0-9_]{0,15}):\s*([^ \t\]]{1,32})`,
			// [REQ]
			`req`, `[`, `]`, `(REQ(?:UEST)?)`,
			// (REQ)
			`req`, `(`, `)`, `(REQ(?:UEST)?)`,
			// {REQ}
			`req`, `{`, `}`, `(REQ(?:UEST)?)`,
			// [ABCD1234]
			`sum`, `[`, `]`, `([0-9A-F]{8})`,
			// [site]
			`site`, `[`, `]`, `([^ \t\]]{1,32})`,
			// -={site}=-
			`site`, `-={`, `}=-`, `([^ \t\}]{1,32})`,
			// {{pass}}
			`pass`, `{{`, `}}`, `([^ \t\}]{1,32})`,
		),
		NewGroupLexer(),
		// --------------- multi ---------------
		NewRegexpLexer(TagTypeSize),
		NewRegexpLexer(TagTypePlatform),
		NewRegexpLexer(TagTypeArch),
		NewRegexpLexer(TagTypeSource),
		NewRegexpLexer(TagTypeResolution),
		NewRegexpSourceLexer(TagTypeCollection),
		NewSeriesLexer(
			// s02, S01E01
			`(?i)^s(?P<s>[0-8]?\d)[\-\._ ]?(?:e(?P<e>\d{1,3}))?\b`,
			// S01S02S03
			`(?i)^(?P<S>(?:s[0-8]?\d){2,4})\b`,
			// 2x1, 1x01
			`(?i)^(?P<s>[0-8]?\d)x(?P<e>\d{1,3})\b`,
			// S01 - 02v3, S07-06, s03-5v.9
			`(?i)^s(?P<s>[0-8]?\d)[\-\._ ]{1,3}(?P<e>\d{1,3})(?:[\-\._ ]{1,3}(?P<v>v\d+(?:\.\d+){0,2}))?\b`,
			// Season.01.Episode.02, Series.01.Ep.02, Series.01, Season.01
			`(?i)^(?:series|season|s)[\-\._ ]?(?P<s>[0-8]?\d)(?:[\-\._ ]?(?:episode|ep)(?P<e>\d{1,3}))?\b`,
			// Vol.1.No.2, vol1no2
			`(?i)^vol(?:ume)?[\-\._ ]?(?P<s>\d{1,3})(?:[\-\._ ]?(?:number|no)[\-\._ ]?(?P<e>\d{1,3}))\b`,
			// Episode 15, E009, Ep. 007, Ep.05-07
			`(?i)^e(?:p(?:isode)?[\-\._ ]{1,3})?(?P<e>\d{1,3})(?:[\-\._ ]{1,3}\d{1,3})?\b`,
			// 10v1.7, 13v2
			`(?i)^(?P<e>\d{1,3})(?P<v>v[\-\._ ]?\d+(?:\.\d){0,2})\b`,
			// S01.Disc02, s01D3, Series.01.Disc.02, S02DVD3
			`(?i)^(?:series|season|s)[\-\._ ]?(?P<s>[0-8]?\d)[\-\._ ]?(?P<d>(?:disc|disk|dvd|d)[\-\._ ]?(?:\d{1,3}))\b`,
		),
		NewVersionLexer(
			// v1.17, v1, v1.2a, v1b
			`(?i)^(?P<v>v[\-\._ ]?\d{1,2}(?:[\._ ]\d{1,2}[a-z]?\d*){0,3})\b`,
			// v2012, v20120803, v20120803, v1999.08.08
			`(?i)^(?P<v>v[\-\._ ]?(?:19|20)\d\d(?:[\-\._ ]?\d\d?){0,2})\b`,
			// v60009
			`(?i)^(?P<v>v[\-\._ ]?\d{4,10})\b`,
		),
		NewDiscSourceYearLexer(
			// VLS2004, 2DVD1999, 4CD2003
			`(?i)^(?P<d>[2-9])?(?P<s>cd|ep|lp|dvd|vls|vinyl)(?P<y>(?:19|20)\d\d)\b`,
			// WEB2007
			`(?i)^(?P<s>web)(?P<y>20\d\d)\b`,
		),
		NewDiscLexer(
			// D01, Disc.1
			`(?i)^(?P<t>d)(?:is[ck][\-\._ ])?(?P<c>\d{1,3})\b`,
			// 12DiSCS
			`(?i)^(?P<c>\d{1,3})[\-\._ ]?di(?P<t>s)[ck]s?\b`,
			// CD1, CD30
			`(?i)^(?P<t>cd)[\-\._ ]?(?P<c>\d{1,2})\b`,
			// DVD2, DVD24 -- does not match DVD5/DVD9
			`(?i)^(?P<t>dvd)[\-\._ ]?(?P<c>[1-46-8]|[12]\d)\b`,
			// 2xVinyl, 3xDVD, 4xCD
			`(?i)^(?P<c>\d{1,2})(?P<t>x(?:cd|ep|lp|dvd|vls|vinyl))\b`,
			// 2Vinyl, 6DVD
			`(?i)^(?P<c>\d{1,2})(?P<x>(?:cd|ep|lp|dvd|vls|vinyl))\b`,
			// CDS3
			`(?i)^(?:(?P<x>cd)s)(?P<c>\d{1,2})\b`,
		),
		NewDateLexer(
			// 2006-01-02, 2006
			`(?i)^(?P<2006>(?:19|20)\d{2})(?:[\-\._ ](?P<01>\d{2})[\-\._ ](?P<02>\d{2}))?\b`,
			// 2006-01
			`(?i)^(?P<2006>(?:19|20)\d{2})?:[\-\._ ](?P<01>\d{2})\b`,
			// 13-02-2006
			`(?i)^(?P<01>\d{2})[\-\._ ](?P<02>\d{2})[\-\._ ](?P<2006>(?:19|20)\d{2})\b`,
			// 02-13-2006
			`(?i)^(?P<02>\d{2})[\-\._ ](?P<01>\d{2})[\-\._ ](?P<2006>(?:19|20)\d{2})\b`,
			// 2nd Jan 2006, 13 Dec 2011, Nov 1999
			`(?i)^(?:(?P<_2>\d{1,2})(?:th|st|nd|rd)?[\-\._ ])?(?P<Jan>Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[\-\._ ](?P<2006>(?:19|20)\d{2})\b`,
			// 01-August-1998
			`(?i)^(?P<_2>\d{1,2})[\-\._ ](?P<January>January|February|March|April|May|June|July|August|September|October|November|December)[\-\._ ](?P<2006>(?:19|20)\d{2})\b`,
			// MAY-30-1992
			`(?i)^(?P<Jan>Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[\-\._ ](?P<_2>\d{1,2})[\-\._ ](?P<2006>(?:19|20)\d{2})\b`,
			// 17.12.15, 20-9-9
			`(?i)^(?P<YY>[12]\d)[\-\._ ](?P<01>\d\d?)[\-\._ ](?P<02>\d\d?)\b`,
		),
		NewRegexpSourceLexer(TagTypeCodec),
		NewAudioLexer(),
		NewRegexpLexer(TagTypeChannels),
		NewRegexpLexer(TagTypeOther),
		NewRegexpLexer(TagTypeCut),
		NewRegexpLexer(TagTypeEdition),
		NewRegexpLexer(TagTypeLanguage),
		NewRegexpLexer(TagTypeRegion),
		NewRegexpLexer(TagTypeContainer),
		NewGenreLexer(),
		NewIDLexer(),
		NewEpisodeLexer(),
	}
}

// NewTrimWhitespaceLexer creates a tag lexer that matches leading and ending
// whitespace.
func NewTrimWhitespaceLexer() Lexer {
	s := `(\t|\n|\f|\r| |⭐|` + string(rune(0xfe0f)) + `)+`
	prefix := regexp.MustCompile(`^` + s)
	suffix := regexp.MustCompile(s + `$`)
	return TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if m := prefix.FindSubmatch(src[i:n]); m != nil {
				start, i = append(start, NewTag(TagTypeWhitespace, nil, m...)), i+len(m[0])
			}
			if m := suffix.FindSubmatch(src[i:n]); m != nil {
				end, n = append(end, NewTag(TagTypeWhitespace, nil, m...)), n-len(m[0])
			}
			return start, end, i, n, true
		},
		Once: true,
	}
}

// NewDateLexer creates a tag lexer for a date.
func NewDateLexer(strs ...string) Lexer {
	lexer := NamedCaptureLexer(strs...)
	return TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if s, v, i, n, ok := lexer(src, buf, i, n); ok {
				// collect year, month, day
				var year, month, day []byte
				matched := true
				for l := 0; l < len(v); l += 2 {
					f, t := string(v[l]), string(v[l+1])
					if f == "YY" {
						f, t = "2006", "20"+t
					}
					if t, err := time.Parse(f, t); err == nil {
						switch f {
						case "06", "2006":
							year = []byte(strconv.Itoa(t.Year()))
						case "_1", "01", "Jan", "January":
							month = []byte(fmt.Sprintf("%02d", t.Month()))
						case "_2", "02":
							day = []byte(fmt.Sprintf("%02d", t.Day()))
						default:
							panic(fmt.Errorf("unknown capture group %q", f))
						}
					} else {
						matched = false
					}
				}
				if matched {
					return append(
						start,
						NewTag(TagTypeDate, nil, s, year, month, day),
					), end, i, n, true
				}
			}
			return start, end, i, n, false
		},
	}
}

// NewSeriesLexer creates a tag lexer for a series.
func NewSeriesLexer(strs ...string) Lexer {
	var f taginfo.FindFunc
	lexer, re, typ := NamedCaptureLexer(strs...), regexp.MustCompile(`(?i)s(\d?\d)`), regexp.MustCompile(`(?i)^disc|disk|dvd|d`)
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			f = taginfo.Find(infos["source"]...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if s, v, i, n, ok := lexer(src, buf, i, n); ok {
				// collect series, episode, version
				var series, episode, version, disc, many []byte
				for l := 0; l < len(v); l += 2 {
					switch string(v[l]) {
					case "s":
						series = v[l+1]
					case "e":
						episode = v[l+1]
					case "v":
						version = v[l+1]
					case "d":
						disc = v[l+1]
					case "S":
						many = []byte(v[l+1])
					default:
						panic(fmt.Errorf("unknown capture group %q", v[l]))
					}
				}
				var tags []Tag
				if len(series) != 0 || len(episode) != 0 {
					if len(version) != 0 {
						s = bytes.TrimSuffix(s, version)
					}
					if len(disc) != 0 {
						s = bytes.TrimSuffix(s, disc)
					}
					tags = append(tags, NewTag(TagTypeSeries, nil, s, series, episode))
				}
				if len(version) != 0 {
					tags = append(tags, NewTag(TagTypeVersion, nil, version, version))
				}
				if len(disc) != 0 {
					disctyp := bytes.ToUpper(disc[:len(typ.FindSubmatch(disc)[0])])
					num := bytes.TrimSpace(disc[len(disctyp):])
					switch string(disctyp) {
					case "DVD":
						tags = append(
							tags,
							NewTag(TagTypeSource, f, disctyp, disctyp),
							NewTag(TagTypeDisc, nil, disc[len(disctyp):], disctyp, num),
						)
					default:
						tags = append(tags, NewTag(TagTypeDisc, nil, disc, disctyp, num))
					}
				}
				// S01S02S03 ...
				if m := re.FindAllSubmatch(many, -1); m != nil {
					for j := 0; j < len(m); j++ {
						tags = append(tags, NewTag(TagTypeSeries, nil, append(m[j], nil)...))
					}
				}
				return append(
					start,
					tags...,
				), end, i, n, true
			}
			return start, end, i, n, false
		},
	}
}

// NewIDLexer creates a tag lexer for a music id.
func NewIDLexer() Lexer {
	alpha, digit, ws := regexp.MustCompile(`[A-Z]`), regexp.MustCompile(`\d`), regexp.MustCompile(`[\-\._ ]`)
	re, lb := regexp.MustCompile(`^([A-Z\d\-\_\. ]{2,24})\)`), regexp.MustCompile(`\([\._ ]{0,2}$`)
	return TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			// lookbehind
			if lb.Match(src[:i]) {
				if m := re.FindSubmatch(buf[i:n]); m != nil {
					a, d, w := alpha.FindAllSubmatch(m[1], -1), digit.FindAllSubmatch(m[1], -1), ws.FindAllSubmatch(m[1], -1)
					switch {
					// ensure enough at least # of alphanumerics
					case a == nil && len(d) > 4 && len(w) < 4,
						len(a) > 1 && len(d) > 1 && (len(a)+len(d) > 4) && len(w) < 4:
						return append(start, NewTag(TagTypeID, nil, src[i:i+len(m[0])], m[1])), end, i + len(m[0]), n, true
					}
				}
			}
			return start, end, i, n, false
		},
	}
}

// NewEpisodeLexer creates a tag lexer for a single episode (`- 2 -`, `- 867 (`, `- 100 [`).
func NewEpisodeLexer() Lexer {
	re, lb := regexp.MustCompile(`^(\d{1,3})(\b|[\._ ]?[\-\[\]\(\)\{\}])`), regexp.MustCompile(`-[\-\._ ]{1,3}$`)
	return TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			// compare against src, and match "lookbehind"
			if lb.Match(src[:i]) {
				if m := re.FindSubmatch(src[i:n]); m != nil {
					tags := []Tag{NewTag(TagTypeSeries, nil, m[1], nil, m[1], nil)}
					if len(m[2]) != 0 {
						tags = append(tags, NewTag(TagTypeDelim, nil, m[2], m[2]))
					}
					return append(start, tags...), end, i + len(m[0]), n, true
				}
			}
			return start, end, i, n, false
		},
	}
}

// NewVersionLexer creates a tag lexer for a version.
func NewVersionLexer(strs ...string) Lexer {
	lexer := NamedCaptureLexer(strs...)
	return TagLexer{
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if s, v, i, n, ok := lexer(src, buf, i, n); ok {
				var version []byte
				for l := 0; l < len(v); l += 2 {
					switch string(v[l]) {
					case "v":
						version = bytes.ToLower(s)
					default:
						panic(fmt.Errorf("unknown capture group %q", v[l]))
					}
				}
				return append(start, NewTag(TagTypeVersion, nil, s, version)), end, i, n, true
			}
			return start, end, i, n, false
		},
	}
}

// NewDiscSourceYearLexer creates a tag lexer for the combined disc, source,
// year style tag.
func NewDiscSourceYearLexer(strs ...string) Lexer {
	var f taginfo.FindFunc
	lexer := NamedCaptureLexer(strs...)
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			f = taginfo.Find(infos["source"]...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if s, v, i, n, ok := lexer(src, buf, i, n); ok {
				// collect series, episode, version
				var disc, source, year []byte
				for l := 0; l < len(v); l += 2 {
					switch string(v[l]) {
					case "d":
						disc = v[l+1]
					case "s":
						source = v[l+1]
					case "y":
						year = v[l+1]
					default:
						panic(fmt.Errorf("unknown capture group %q", v[l]))
					}
				}
				var tags []Tag
				if len(disc) != 0 {
					tags = append(tags, NewTag(TagTypeDisc, nil, disc, []byte{'X'}, disc))
				}
				tags = append(
					tags,
					NewTag(TagTypeSource, f, source, source),
					NewTag(TagTypeDate, nil, year, year, nil, nil),
				)
				return append(start, tags...), end, i + len(s), n, true
			}
			return start, end, i, n, false
		},
	}
}

// NewDiscLexer creates a tag lexer for a disc.
//
//	n - number
//	t - type
//	x - xXtype
func NewDiscLexer(strs ...string) Lexer {
	var f taginfo.FindFunc
	lexer, re := NamedCaptureLexer(strs...), regexp.MustCompile(`(?i)^dvd|cd|d|s|x`)
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			f = taginfo.Find(infos["source"]...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if s, v, j, k, ok := lexer(src, buf, i, n); ok {
				var typ, c, x []byte
				for l := 0; l < len(v); l += 2 {
					switch string(v[l]) {
					case "c":
						c = v[l+1]
					case "t":
						typ = bytes.ToUpper(v[l+1])
					case "x":
						x = bytes.ToUpper(v[l+1])
					default:
						panic(fmt.Errorf("unknown capture group %q", v[l]))
					}
				}
				if len(typ) != 0 {
					typ = bytes.ToUpper(typ[:len(re.FindSubmatch(s)[0])])
				}
				switch string(typ) {
				case "D", "S":
					return append(start, NewTag(TagTypeDisc, nil, s, typ, c)), end, j, k, true
				case "DVD", "CD":
					return append(start,
						NewTag(TagTypeSource, f, s[:len(typ)], typ, typ),
						NewTag(TagTypeDisc, nil, s[len(typ):], typ, c),
					), end, j, k, true
				case "X":
					return append(start,
						NewTag(TagTypeDisc, nil, s[:len(c)+1], typ, c),
						NewTag(TagTypeSource, f, s[len(c)+1:], s[len(c)+1:]),
					), end, j, k, true
				case "":
					return append(start,
						NewTag(TagTypeDisc, nil, s[:len(c)+1], []byte{'X'}, c),
						NewTag(TagTypeSource, f, s[len(c)+1:], x),
					), end, j, k, true
				default:
					panic(fmt.Errorf("unknown type %q", typ))
				}
			}
			return start, end, i, n, false
		},
	}
}

// NewAudioLexer creates a tag lexer for audios.
func NewAudioLexer() Lexer {
	var re *regexp.Regexp
	var audiof, channelsf taginfo.FindFunc
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			audio, channels := infos["audio"], infos["channels"]
			var v []string
			for _, info := range channels {
				v = append(v, strings.ReplaceAll(info.Tag(), `.`, `[\._ ]?`))
			}
			re = regexp.MustCompile(reutil.Taginfo(`^i`, audio...) + `(?:[\-\._ ]?(` + strings.Join(v, "|") + `))?(?:\b|[\-\._ ])`)
			audiof, channelsf = taginfo.Find(audio...), taginfo.Find(channels...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if m := re.FindSubmatch(src[i:n]); m != nil {
				l := len(m[0])
				if len(m[2]) != 0 {
					m[0] = bytes.TrimSuffix(m[0], m[2])
				}
				start = append(start, NewTag(TagTypeAudio, audiof, m[0], m[1]))
				if len(m[2]) != 0 {
					start = append(start, NewTag(TagTypeChannels, channelsf, m[2], m[2]))
				}
				return start, end, i + l, n, true
			}
			return start, end, i, n, false
		},
	}
}

// NewGenreLexer creates a tag lexer for a genre.
func NewGenreLexer() Lexer {
	var f taginfo.FindFunc
	var re, lb, other *regexp.Regexp
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			genre := infos["genre"]
			// build regexp for (Genre)
			var v, tagv []string
			for _, info := range genre {
				v = append(v, info.RE())
				if other := info.Other(); other != "" {
					tagv = append(tagv, other)
				}
			}
			s := `\(?(` + strings.Join(v, `|`) + `)\s*\)`
			re, lb, other, f = regexp.MustCompile(`(?i)^`+s), regexp.MustCompile(`(?i)\(\s*`+s+`$`), regexp.MustCompile(`(?i)^(`+strings.Join(tagv, `|`)+`)\b`), taginfo.Find(genre...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			var m [][]byte
			// compare against src, and match "lookbehind"
			if m = re.FindSubmatch(src[i:n]); m != nil && lb.Match(src[:i+len(m[0])]) {
				return append(
					start,
					NewTag(TagTypeGenre, f, m...),
				), end, i + len(m[0]), n, true
			} else if m = other.FindSubmatch(buf[i:n]); m != nil {
				return append(
					start,
					NewTag(TagTypeGenre, f, m...),
				), end, i + len(m[0]), n, true
			}
			return start, end, i, n, false
		},
	}
}

// NewGroupLexer creates a tag lexer for a group.
func NewGroupLexer() Lexer {
	const delim, invalid = '-', ` _.()[]+`
	year, group := regexp.MustCompile(`\b(19|20)\d{2}\b`), regexp.MustCompile(`(?i)^[a-z_ ]{4,10}$`)
	var groupf, otherf taginfo.FindFunc
	var re, special *regexp.Regexp
	var shortTags map[string]bool
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, short map[string]bool) {
			var v []string
			group, other := infos["group"], infos["other"]
			for _, info := range other {
				if s := info.Other(); s != "" {
					v = append(v, s)
				}
			}
			groupf, otherf = taginfo.Find(group...), taginfo.Find(other...)
			re, special = regexp.MustCompile(`(?i)[\-\._ ]+`+reutil.Taginfo(`$`, group...)), regexp.MustCompile(`(?i)_(`+strings.Join(v, `|`)+`)$`)
			shortTags = short
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			// special end tags on groups
			if m := special.FindSubmatch(src[i:n]); m != nil {
				end, n = append(end, NewTag(TagTypeOther, otherf, m...)), n-len(m[0])
			}
			// known groups
			if m := re.FindSubmatch(src[i:n]); m != nil {
				return start, append(end, NewTag(TagTypeGroup, groupf, m...)), i, n - len(m[0]), true
			}
			// clamp to last year
			l := i
			if m := year.FindAllSubmatchIndex(buf[l:n], -1); m != nil {
				l = m[0][1]
			}
			// locate delimiter and check valid group
			if j := bytes.LastIndexByte(buf[l:n], delim); j != -1 {
				s := src[l+j+1 : n]
				if grp := bytes.Trim(s, " \t_"); len(grp) != 0 && (!bytes.ContainsAny(s, invalid) || (len(s) <= 14 && group.Match(grp))) {
					if !shortTags[string(grp)] {
						return start, append(
							end,
							NewTag(TagTypeGroup, nil, src[l+j+1:n], grp),
							NewTag(TagTypeDelim, nil, src[l+j:l+j+1], []byte{delim}),
						), i, l + j, false
					}
				}
			}
			return start, end, i, n, false
		},
		Once: true,
	}
}

// NewMetaLexer creates a tag lexer for a file's meta data.
func NewMetaLexer(strs ...string) Lexer {
	if len(strs)%4 != 0 {
		panic("must be divisible by 4")
	}
	o := len(strs) / 4
	// build prefix, suffix
	prefix, suffix, hasTwo := make([]*regexp.Regexp, o), make([]*regexp.Regexp, o), make([]bool, o)
	for l := 0; l < o; l++ {
		s := `\s*\Q` + strs[l*4+1] + `\E\s*` + strs[l*4+3] + `\s*\Q` + strs[l*4+2] + `\E\s*`
		prefix[l], suffix[l] = regexp.MustCompile(`^`+s), regexp.MustCompile(s+`$`)
		if n := prefix[l].NumSubexp(); n != 1 && n != 2 {
			panic("must have exactly 1 or 2 capture groups")
		}
		hasTwo[l] = prefix[l].NumSubexp() == 2
	}
	var delim *regexp.Regexp
	var shortTags map[string]bool
	return TagLexer{
		Init: func(_ map[string][]*taginfo.Taginfo, re *regexp.Regexp, short map[string]bool) {
			delim, shortTags = re, short
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			var l int
			var d []byte
			var m [][]byte
			var k string
			var v []byte
			var matched, short bool
			prev := make(map[string]bool, o)
			// prefixes
			for ; i < n; i++ {
				matched = false
				for l = 0; l < o && !matched; l++ {
					if m = prefix[l].FindSubmatch(src[i:n]); m != nil {
						if k, v = strs[l*4], m[1]; hasTwo[l] {
							k, v = string(m[1]), m[2]
						}
						short = len(strs[l*4+1]) == 1 && shortTags[strings.ToUpper(string(v))]
						matched, prev[k] = !prev[k] && !short && !bytes.ContainsAny(v, "\t\r\n\f +"), true
					}
				}
				if matched {
					if len(d) != 0 {
						start, d = append(start, NewTag(TagTypeDelim, nil, d, d)), d[:0]
					}
					start = append(start, NewTag(TagTypeMeta, nil, m[0], []byte(k), v))
					i = i + len(m[0]) - 1
				} else if !delim.Match(src[i : i+1]) {
					break
				} else {
					d = append(d, src[i])
				}
			}
			// backtrack remaining
			if len(d) != 0 {
				i, d = i-len(d), d[:0]
			}
			// suffixes
			for ; i < n; n-- {
				matched = false
				for l = 0; l < o && !matched; l++ {
					if m = suffix[l].FindSubmatch(src[i:n]); m != nil {
						if k, v = strs[l*4], m[1]; hasTwo[l] {
							k, v = string(m[1]), m[2]
						}
						short = len(strs[l*4+1]) == 1 && shortTags[strings.ToUpper(string(v))]
						matched, prev[k] = !prev[k] && !short && !bytes.ContainsAny(v, "\t\r\n\f +"), true
					}
				}
				if matched {
					if len(d) != 0 {
						end, d = append(end, NewTag(TagTypeDelim, nil, d, d)), d[:0]
					}
					end = append(end, NewTag(TagTypeMeta, nil, m[0], []byte(k), v))
					n = n - len(m[0]) + 1
				} else if !delim.Match(src[n-1 : n]) {
					break
				} else {
					d = append([]byte{src[n-1]}, d...)
				}
			}
			// backtrack remaining
			if len(d) != 0 {
				end = append(end, NewTag(TagTypeDelim, nil, d, d))
			}
			return start, end, i, n, true
		},
		Once: true,
	}
}

// NewExtLexer creates a tag lexer for a file's extension.
func NewExtLexer() Lexer {
	var f taginfo.FindFunc
	var re *regexp.Regexp
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			ext := infos["ext"]
			re, f = regexp.MustCompile(`(?i:\.`+reutil.Taginfo("$", ext...)+`)`), taginfo.Find(ext...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if m := re.FindSubmatch(src[i:n]); m != nil {
				return start, append(end, NewTag(TagTypeExt, f, m...)), i, n - len(m[0]), true
			}
			return start, end, i, n, false
		},
		Once: true,
	}
}

// NewRegexpLexer creates a tag lexer for a regexp.
func NewRegexpLexer(typ TagType) TagLexer {
	var f taginfo.FindFunc
	var re *regexp.Regexp
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			info := infos[strings.ToLower(typ.String())]
			re, f = regexp.MustCompile(reutil.Taginfo(`^ib`, info...)), taginfo.Find(info...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if m := re.FindSubmatch(buf[i:n]); m != nil {
				return append(start, NewTag(typ, f, append([][]byte{src[i : i+len(m[0])]}, m[1:]...)...)), end, i + len(m[0]), n, true
			}
			return start, end, i, n, false
		},
	}
}

// NewRegexpSourceLexer creates a tag lexer for a regexp.
func NewRegexpSourceLexer(typ TagType) TagLexer {
	var f taginfo.FindFunc
	var re *regexp.Regexp
	return TagLexer{
		Init: func(infos map[string][]*taginfo.Taginfo, _ *regexp.Regexp, _ map[string]bool) {
			info := infos[strings.ToLower(typ.String())]
			re, f = regexp.MustCompile(reutil.Taginfo("^i", info...)+`(?:\b|[\-\._ ])`), taginfo.Find(info...)
		},
		Lex: func(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int, bool) {
			if m := re.FindSubmatch(src[i:n]); m != nil {
				if len(m[0]) != len(m[1]) {
					v, delim := src[i:i+len(m[1])], src[i+len(m[1]):i+len(m[0])]
					return append(
						start,
						NewTag(typ, f, [][]byte{v, v}...),
						NewTag(TagTypeDelim, nil, [][]byte{delim, delim}...),
					), end, i + len(m[0]), n, true
				}
				return append(start, NewTag(typ, f, append([][]byte{src[i : i+len(m[0])]}, m[1:]...)...)), end, i + len(m[0]), n, true
			}
			return start, end, i, n, false
		},
	}
}

// NamedCaptureLexer returns a func that matches named capture groups,
// returning as name/value string pairs.
func NamedCaptureLexer(strs ...string) func([]byte, []byte, int, int) ([]byte, [][]byte, int, int, bool) {
	var regexps []*regexp.Regexp
	var indexes [][]int
	var subexps [][]string
	for l := 0; l < len(strs); l++ {
		// build regexp and collect subexp indexes
		re := regexp.MustCompile(strs[l])
		var idx []int
		var names []string
		for j, name := range re.SubexpNames() {
			if name != "" {
				idx, names = append(idx, j), append(names, name)
			}
		}
		if len(idx) != 0 {
			regexps, indexes, subexps = append(regexps, re), append(indexes, idx), append(subexps, names)
		}
	}
	o := len(regexps)
	return func(src, buf []byte, i, n int) ([]byte, [][]byte, int, int, bool) {
		for l := 0; l < o; l++ {
			if m := regexps[l].FindSubmatch(buf[i:n]); m != nil {
				// build values
				var v [][]byte
				for j, k := range indexes[l] {
					if len(m[k]) != 0 {
						v = append(v, []byte(subexps[l][j]), m[k])
					}
				}
				if len(v) != 0 {
					return src[i : i+len(m[0])], v, i + len(m[0]), n, true
				}
			}
		}
		return nil, nil, i, n, false
	}
}
