package rls

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/moistari/rls/reutil"
	"github.com/moistari/rls/taginfo"
)

// TagParser is a release tag parser.
type TagParser struct {
	builder Builder
	delim   *regexp.Regexp
	ellip   []byte
	work    *regexp.Regexp
	once    []LexFunc
	multi   []LexFunc
}

// NewTagParser creates a new release tag parser.
func NewTagParser(infos map[string][]*taginfo.Taginfo, lexers ...Lexer) Parser {
	// delims
	var v []string
	for r := rune(0); r < 255; r++ {
		if isAnyDelim(r) {
			v = append(v, string(r))
		}
	}
	delim := regexp.MustCompile(`^((?:` + reutil.Join(true, v...) + ")+)")
	// build short tags
	short := make(map[string]bool)
	for _, v := range infos {
		for _, info := range v {
			for _, field := range strings.FieldsFunc(info.Tag(), isAnyDelim) {
				if len(field) < 5 && !strings.Contains(field, "$") {
					short[strings.ToUpper(field)] = true
				}
			}
		}
	}
	// separate once and multi
	var once, multi []LexFunc
	for _, lexer := range lexers {
		if f, o := lexer.Initialize(infos, delim, short); o {
			once = append(once, f)
		} else {
			multi = append(multi, f)
		}
	}
	// init builder
	builder := DefaultBuilder
	if b, ok := builder.(interface {
		Init(map[string][]*taginfo.Taginfo) Builder
	}); ok {
		builder = b.Init(infos)
	}
	return &TagParser{
		builder: builder,
		delim:   delim,
		ellip:   []byte("..."),
		work:    regexp.MustCompile(`[_,\+]`),
		once:    once,
		multi:   multi,
	}
}

// SetBuilder sets the builder for the tag parser.
func (p *TagParser) SetBuilder(builder Builder) {
	p.builder = builder
}

// Parse parses tags in buf.
func (p *TagParser) Parse(src []byte) ([]Tag, int) {
	// working buf
	buf := p.work.ReplaceAll(src, []byte{' '})
	i, n := 0, len(buf)
	var start, end []Tag
	// once
	for _, f := range p.once {
		start, end, i, n, _ = f(src, buf, start, end, i, n)
	}
	// consume
	for i < n {
		start, end, i, _ = p.next(src, buf, start, end, i, n)
	}
	// add end (reversed) to start
	tags := make([]Tag, len(start)+len(end))
	copy(tags, start)
	for i := 0; i < len(end); i++ {
		tags[len(start)+i] = end[len(end)-i-1]
	}
	return tags, len(tags) - len(end)
}

// next reads the next token from src up to the next delimiter. Iterates over
// the lexers until a match occurs. If none of the lexers match, then iterates
// over src, capturing all text, until src is exhausted or until a delimiter is
// encountered. Appends captured tags to start or end (where appropriate)
// returning the modified slices and new values of i, n.
//
// Lexers have the choice of matching against src or buf. Buf is the working
// version of src, with underscores and other runes replaced with spaces. Using
// buf allows lexers to matching with Go regexp `\b`.
func (p *TagParser) next(src, buf []byte, start, end []Tag, i, n int) ([]Tag, []Tag, int, int) {
	// delimiter
	if bytes.HasPrefix(src[i:n], p.ellip) {
		return append(
			start,
			NewTag(TagTypeDelim, nil, p.ellip, p.ellip),
		), end, i + len(p.ellip), n
	} else if m := p.delim.FindSubmatch(src[i:n]); m != nil {
		return append(
			start,
			NewTag(TagTypeDelim, nil, m[0], m[1]),
		), end, i + len(m[0]), n
	}
	// run all lexers
	for _, f := range p.multi {
		if s, e, j, k, ok := f(src, buf, start, end, i, n); ok {
			return s, e, j, k
		}
	}
	// text
	j := i
	for j < n && !p.delim.Match(src[j:]) {
		j++
	}
	return append(
		start,
		NewTag(TagTypeText, nil, src[i:j], src[i:j]),
	), end, j, n
}

// ParseRelease parses a release from src.
func (p *TagParser) ParseRelease(src []byte) Release {
	return p.builder.Build(p.Parse(src))
}

// TagBuilder is a release builder.
type TagBuilder struct {
	// missing finds acronyms without periods.
	missing *regexp.Regexp
	// bad finds bad acronyms with periods.
	bad *regexp.Regexp
	// fix fixes issues with acronyms.
	fix *regexp.Regexp
	// spaces matches multiple spaces.
	spaces *regexp.Regexp
	// ellips matches multiple 3 or more periods (an ellipsis...).
	ellips *regexp.Regexp
	// plus matches pluses.
	plus *regexp.Regexp
	// sum matches a file checksum.
	sum *regexp.Regexp
	// digits matches all digits.
	digits *regexp.Regexp
	// digpre matches digit prefixes.
	digpre *regexp.Regexp
	// digsuf matches digit suffixes.
	digsuf *regexp.Regexp
	// infos are tag info.
	infos map[string][]*taginfo.Taginfo
	// containerf is the container find func.
	containerf taginfo.FindFunc
}

// NewTagBuilder creates a new release builder.
func NewTagBuilder() *TagBuilder {
	return &TagBuilder{
		missing: regexp.MustCompile(`\b[A-Z][\. ][A-Z](?:[\. ][A-Z])*[\. ]?\b`),
		bad:     regexp.MustCompile(`[^A-Z][-\. ][A-Z]\.($|[^A-Z])`),
		fix:     regexp.MustCompile(`([A-Z])\.`),
		spaces:  regexp.MustCompile(`\s+`),
		ellips:  regexp.MustCompile(`\.{3,}`),
		plus:    regexp.MustCompile(`(\+)`),
		sum:     regexp.MustCompile(`(?i)^[a-f0-9]{8}$`),
		digits:  regexp.MustCompile(`^\d+$`),
		digpre:  regexp.MustCompile(`^\d+`),
		digsuf:  regexp.MustCompile(`\d+$`),
	}
}

// Init creates a new builder using the provided infos.
func (b *TagBuilder) Init(infos map[string][]*taginfo.Taginfo) Builder {
	return &TagBuilder{
		missing:    b.missing,
		bad:        b.bad,
		fix:        b.fix,
		spaces:     b.spaces,
		ellips:     b.ellips,
		plus:       b.plus,
		sum:        b.sum,
		digits:     b.digits,
		digpre:     b.digpre,
		digsuf:     b.digsuf,
		infos:      infos,
		containerf: taginfo.Find(infos["container"]...),
	}
}

// Build builds a release from tags.
func (b *TagBuilder) Build(tags []Tag, end int) Release {
	r := &Release{
		tags: tags,
		end:  end,
	}
	// initialize / fix tags
	b.init(r)
	// collect tags into release
	b.collect(r)
	// guess type
	r.Type = b.inspect(r)
	// special
	b.specialDate(r)
	// unset exclusive tags
	b.unset(r)
	// read titles
	i := b.titles(r)
	// demarcate unused
	b.unused(r, i)
	return *r
}

// init fixes the initial tag set.
func (b *TagBuilder) init(r *Release) {
	// determine earliest pivot
	m, pivot := b.pivots(r, TagTypeDate, TagTypeSource, TagTypeSeries, TagTypeResolution, TagTypeVersion)
	date, series := m[TagTypeDate], m[TagTypeSeries]
	// reset/collect dates
	if date != -1 {
		r.dates = append(r.dates, date)
	}
	if dates := b.reset(r, date, TagTypeDate); len(dates) != 0 {
		r.dates = append(r.dates, dates...)
	}
	// fix "amazon" and "md" matches before a series/date tag
	if date != -1 || series != -1 {
		i := min(date, series)
		switch {
		case date == -1:
			i = series
		case series == -1:
			i = date
		}
		b.fixSpecial(r, i)
	}
	// get first text prior to pivot
	end := b.end(r, pivot)
	// reset language/other/arch/platform prior to end
	_ = b.reset(r, end, TagTypeLanguage, TagTypeOther, TagTypeArch, TagTypePlatform)
	b.fixFirst(r)
	start := b.start(r, 0)
	b.fixBad(r, start, end)
	b.fixNoText(r, end)
	b.fixIsolated(r)
	b.fixMusic(r)
}

// pivots finds the last position for the specified tags, generating a map of
// the types and earliest position of all.
func (b *TagBuilder) pivots(r *Release, types ...TagType) (map[TagType]int, int) {
	m := make(map[TagType]int, len(types))
	for _, typ := range types {
		m[typ] = -1
	}
	j := -1
	// find minimum of types
	for i := r.end; i > 0; i-- {
		if typ := r.tags[i-1].TagType(); typ.Is(types...) && m[typ] == -1 {
			m[typ], j = i-1, i-1
		}
	}
	if j != -1 {
		return m, j
	}
	return m, r.end
}

// reset any tags of types prior to i.
func (b *TagBuilder) reset(r *Release, i int, types ...TagType) []int {
	// reset date tag and collect indexes
	var v []int
	for ; i > 0; i-- {
		if r.tags[i-1].Is(types...) {
			r.tags[i-1], v = r.tags[i-1].As(TagTypeText, nil), append(v, i-1)
		}
	}
	return v
}

// start finds the first text tag after i.
func (b *TagBuilder) start(r *Release, i int) int {
	for ; i < r.end && !r.tags[i].Is(TagTypeText); i++ {
	}
	return i
}

// end finds the first text tag before i.
func (b *TagBuilder) end(r *Release, i int) int {
	for ; i > 0 && !r.tags[i-1].Is(TagTypeText); i-- {
	}
	return i
}

// fixFirst fixes the first non text tag if it was badly matched.
func (b *TagBuilder) fixFirst(r *Release) {
	var i int
	// seek
	for ; i < r.end && r.tags[i].Is(TagTypeWhitespace, TagTypeDelim); i++ {
	}
	if i != r.end && r.tags[i].Is(
		TagTypeCut,
		TagTypeEdition,
		TagTypeOther,
		TagTypeSource,
		TagTypePlatform,
		TagTypeArch,
	) {
		r.tags[i] = r.tags[i].As(TagTypeText, nil)
	}
}

// fixBad fixes bad collection/language/other/arch/platform tags before i.
func (b *TagBuilder) fixBad(r *Release, start, i int) {
	// seek non language/edition/collection/cut/other/source/delim
	for ; i > start &&
		r.tags[i-1].Is(
			TagTypeLanguage,
			TagTypeEdition,
			TagTypeCut,
			TagTypeOther,
			TagTypeCollection,
			TagTypeDelim,
			TagTypeSource,
		); i-- {
	}
	// fix collection/language/other/arch/platform tag between start and i
	for ; i > start; i-- {
		switch {
		case r.tags[i-1].Is(TagTypeCollection) && r.tags[i-1].Collection() == "IMAX":
			// ignore imax
		case r.tags[i-1].Is(
			TagTypeCollection,
			TagTypeLanguage,
			TagTypeOther,
			TagTypeArch,
			TagTypePlatform,
		):
			r.tags[i-1] = r.tags[i-1].As(TagTypeText, nil)
		}
	}
}

// fixSpecial fixes special collection and other tags before i.
func (b *TagBuilder) fixSpecial(r *Release, i int) {
	for ; i > 0; i-- {
		typ, c, o, s := r.tags[i-1].TagType(), r.tags[i-1].Collection(), r.tags[i-1].Other(), strings.ToLower(r.tags[i-1].Text())
		switch {
		case typ == TagTypeCollection && c == "AMZN" && s == "amazon",
			typ == TagTypeCollection && c == "CC",
			typ == TagTypeCollection && c == "RED",
			typ == TagTypeSource && r.tags[i-1].Text() == "Web",
			typ == TagTypeCut && r.tags[i-1].Text() == "Uncut",
			typ == TagTypeOther && o == "MD":
			r.tags[i-1] = r.tags[i-1].As(TagTypeText, nil)
		}
	}
}

// fixNoText fixes having no text but has a collection tag.
func (b *TagBuilder) fixNoText(r *Release, end int) {
	// bail if any text tags
	i, n := 0, min(end+1, len(r.tags))
	for ; i < n; i++ {
		if r.tags[i].Is(TagTypeText) {
			return
		}
	}
	// reset
	for i = 0; i < n; i++ {
		if r.tags[i].Is(TagTypeCollection) {
			r.tags[i] = r.tags[i].As(TagTypeText, nil)
		}
	}
}

// fixIsolated fixes isolated collection/language/other/arch/platform tags.
func (b *TagBuilder) fixIsolated(r *Release) {
	// if tag is isolated with text on both sides, change to text
	for i := r.end - 1; i > 1; i-- {
		if r.tags[i-1].Is(
			TagTypeCollection,
			TagTypeLanguage,
			TagTypeOther,
			TagTypeArch,
			TagTypePlatform,
		) && isolated(r.tags[:r.end], i-1, -1) && isolated(r.tags[:r.end], i-1, +1) {
			r.tags[i-1] = r.tags[i-1].As(TagTypeText, nil)
		}
	}
}

// fixMusic fixes music tags. Changes single cbr tag to the comic tag, and
// converts to text bootleg tag that is not surrounded by '-' or '()'.
func (b *TagBuilder) fixMusic(r *Release) {
	// when only one music tag of `cbr`, change to container `cbr` (comic)
	count, isCbr := 0, false
	var pos int
	for i := 0; i < r.end; i++ {
		// count audio tags and position of cbr tag
		if r.tags[i].Is(TagTypeAudio) {
			if r.tags[i].Audio() == "CBR" {
				isCbr, pos = true, i
			}
			count++
		}
		// reset bootleg tag that is not '-bootleg-' or '(bootleg)'
		if i != 0 && r.tags[i].Is(TagTypeOther) && r.tags[i].Other() == "BOOTLEG" {
			wrapped := (peek(r.tags, i-1, TagTypeDelim) && strings.HasSuffix(r.tags[i-1].Delim(), "-") &&
				peek(r.tags, i+1, TagTypeDelim) && strings.HasPrefix(r.tags[i+1].Delim(), "-")) ||
				(peek(r.tags, i-1, TagTypeDelim) && strings.HasSuffix(r.tags[i-1].Delim(), "(") &&
					peek(r.tags, i+1, TagTypeDelim) && strings.HasPrefix(r.tags[i+1].Delim(), ")"))
			if !wrapped {
				r.tags[i] = r.tags[i].As(TagTypeText, nil)
			}
		}
	}
	// reset single cbr tag
	if count == 1 && isCbr {
		r.tags[pos] = r.tags[pos].As(TagTypeContainer, b.containerf)
	}
}

// collect collects the collect into the release.
func (b *TagBuilder) collect(r *Release) {
	for i := 0; i < len(r.tags); i++ {
		switch r.tags[i].typ {
		case TagTypeText:
		case TagTypePlatform:
			if r.Platform == "" {
				r.Platform = r.tags[i].Platform()
			}
		case TagTypeArch:
			if r.Arch == "" {
				r.Arch = r.tags[i].Arch()
			}
		case TagTypeSource:
			// stomping allowed when more precise source available
			if s := r.tags[i].Source(); r.Source == "" || r.Source == "CD" || (r.Source == "DVD" && s != "CD") {
				r.Source = s
			}
		case TagTypeResolution:
			if r.Resolution == "" {
				r.Resolution = r.tags[i].Resolution()
			}
		case TagTypeCollection:
			if r.Collection == "" {
				r.Collection = r.tags[i].Collection()
			}
		case TagTypeDate:
			r.Year, r.Month, r.Day = r.tags[i].Date()
		case TagTypeSeries:
			series, episode := r.tags[i].Series()
			if r.Series == 0 {
				r.Series = series
			}
			if r.Episode == 0 {
				r.Episode = episode
			}
		case TagTypeVersion:
			if r.Version == "" {
				r.Version = r.tags[i].Version()
			}
		case TagTypeDisc:
			if r.Disc == "" {
				r.Disc = fmt.Sprintf("%s", r.tags[i])
			}
		case TagTypeCodec:
			r.Codec = append(r.Codec, r.tags[i].Codec())
		case TagTypeAudio:
			r.Audio = append(r.Audio, r.tags[i].Audio())
		case TagTypeChannels:
			if r.Channels == "" {
				r.Channels = r.tags[i].Channels()
			}
		case TagTypeOther:
			r.Other = append(r.Other, r.tags[i].Other())
		case TagTypeCut:
			r.Cut = append(r.Cut, r.tags[i].Cut())
		case TagTypeEdition:
			r.Edition = append(r.Edition, r.tags[i].Edition())
		case TagTypeLanguage:
			r.Language = append(r.Language, r.tags[i].Language())
		case TagTypeSize:
			if r.Size == "" {
				r.Size = r.tags[i].Size()
			}
		case TagTypeRegion:
			if r.Region == "" {
				r.Region = r.tags[i].Region()
			}
		case TagTypeContainer:
			if r.Container == "" {
				r.Container = r.tags[i].Container()
			}
		case TagTypeGenre:
			if r.Genre == "" {
				r.Genre = r.tags[i].Genre()
			}
		case TagTypeID:
			if r.ID == "" {
				r.ID = r.tags[i].ID()
			}
		case TagTypeGroup:
			r.Group = r.tags[i].Group()
		case TagTypeMeta:
			switch k, v := r.tags[i].Meta(); {
			case k == "site" && r.Site == "":
				r.Site = v
			case k == "sum" && r.Sum == "":
				r.Sum = v
			case k == "pass" && r.Pass == "":
				r.Pass = v
			case k == "req":
				r.Req = true
			default:
				r.Meta = append(r.Meta, k+":"+v)
			}
		case TagTypeExt:
			r.Ext = r.tags[i].Ext()
		}
	}
	// collect year, month, day from unset date tags
	for i := len(r.dates); i > 0; i-- {
		year, month, day := r.tags[r.dates[i-1]].Date()
		if r.Year == 0 && year != 0 {
			r.Year = year
		}
		if r.Month == 0 && month != 0 {
			r.Month = month
		}
		if r.Day == 0 && day != 0 {
			r.Day = day
		}
	}
}

// inspect inspects the release, returning its expected type.
func (b *TagBuilder) inspect(r *Release) Type {
	if r.Type != Unknown {
		return r.Type
	}
	// inspect types
	var app, series, movie bool
	for i := len(r.tags); i > 0; i-- {
		typ := r.tags[i-1].InfoType()
		app, series, movie = app || typ == App, series || r.tags[i-1].Is(TagTypeSeries), movie || typ == Movie
		switch typ {
		case Book, Game:
			// peek for comic, education, magazine
			for j := i - 1; j > 0; j-- {
				if typ := r.tags[j-1].InfoType(); typ.Is(Comic, Education, Magazine) {
					return typ
				}
			}
			return typ
		case Series, Episode:
			if r.Episode != 0 || (r.Series == 0 && r.Episode == 0) && !contains(r.Other, "BOXSET") {
				return Episode
			}
			return Series
		case Education:
			if r.Series == 0 && r.Episode == 0 {
				return Education
			}
		case Music:
			// peek for audiobook
			for j := i - 1; j > 0; j-- {
				if typ := r.tags[j-1].InfoType(); typ.Is(Audiobook) {
					return typ
				}
			}
			return typ
		case Audiobook, Comic, Magazine:
			return typ
		}
		// exclusive tag not superseded by version/episode/date
		if r.tags[i-1].InfoExcl() &&
			r.Version == "" &&
			r.Series == 0 && r.Episode == 0 &&
			r.Day == 0 && r.Month == 0 {
			return typ
		}
	}
	// check music style tag delimiters
	for count, i := 0, len(r.tags)-1; i > 1; i-- {
		if r.tags[i-1].Is(TagTypeDate, TagTypeCodec, TagTypeAudio, TagTypeResolution, TagTypeSource, TagTypeLanguage) &&
			peek(r.tags, i-2, TagTypeDelim) && strings.HasSuffix(r.tags[i-2].Delim(), "-") &&
			peek(r.tags, i, TagTypeDelim) && strings.HasPrefix(r.tags[i].Delim(), "-") {
			count++
			if count > 1 {
				return Music
			}
		}
	}
	// defaults
	switch {
	case r.Episode != 0 || (r.Year != 0 && r.Month != 0 && r.Day != 0):
		return Episode
	case r.Series != 0 || series:
		return Series
	case app || (r.Version != "" && r.Resolution == ""):
		return App
	case movie || r.Resolution != "":
		return Movie
	case (r.Source == "" || r.Source == "WEB") && r.Resolution == "" && r.Year != 0:
		return Music
	}
	return Unknown
}

// specialDate handles special dates.
func (b *TagBuilder) specialDate(r *Release) {
	// on magazines, check prior to the date if there is a month listed
	if r.Type == Magazine && r.Year != 0 && r.Month == 0 && r.Day == 0 {
		// seek to prior text
		i := r.dates[0] - 1
		for ; i > 0 && r.tags[i].Is(TagTypeDelim); i-- {
		}
		// reset tag to date if tag is text, and parses as month name
		if i >= 0 && r.tags[i].Is(TagTypeText) {
			s := r.tags[i].Text()
			if t, err := time.Parse("January", s); err == nil {
				r.Month = int(t.Month())
				year, month := strconv.Itoa(r.Year), strconv.Itoa(r.Month)
				r.tags[i] = NewTag(TagTypeDate, nil, []byte(s), []byte(year), []byte(month), nil)
				r.dates = append(r.dates, i)
			}
		}
	}
}

// unset unsets exclusive and other misrecognized tags on the release.
func (b *TagBuilder) unset(r *Release) {
	movieSeriesEpisodeMusicGame := r.Type.Is(Movie, Series, Episode, Music, Game)
	for i := 0; i < len(r.tags); i++ {
		ityp := r.tags[i].InfoType()
		// reset exclusive tags
		if ityp != r.Type && r.tags[i].Is(
			TagTypePlatform,
			TagTypeArch,
			TagTypeSource,
			TagTypeResolution,
			TagTypeCollection,
			TagTypeCodec,
			TagTypeAudio,
			TagTypeChannels,
			TagTypeOther,
			TagTypeCut,
			TagTypeEdition,
			TagTypeLanguage,
			TagTypeSize,
			TagTypeRegion,
			TagTypeContainer,
			TagTypeGenre,
			TagTypeGroup,
			TagTypeExt,
		) && r.tags[i].InfoExcl() {
			switch typ, s := r.tags[i].TagType(), r.tags[i].Normalize(); {
			case typ == TagTypePlatform && r.Platform == s && !contains(r.Other, "Strategy.Guide"):
				r.Platform, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeArch && r.Arch == s:
				r.Arch, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeSource && r.Source == s:
				r.Source, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeResolution && r.Resolution == s:
				r.Resolution, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeCollection && r.Collection == s:
				r.Collection, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeCodec && contains(r.Codec, s):
				r.Codec, r.tags[i] = remove(r.Codec, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeAudio && contains(r.Audio, s):
				r.Audio, r.tags[i] = remove(r.Audio, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeChannels && r.Channels == s:
				r.Channels, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeOther && contains(r.Other, s):
				r.Other, r.tags[i] = remove(r.Other, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeCut && contains(r.Cut, s):
				r.Cut, r.tags[i] = remove(r.Cut, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeEdition && contains(r.Edition, s):
				r.Edition, r.tags[i] = remove(r.Edition, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeLanguage && contains(r.Language, s):
				r.Language, r.tags[i] = remove(r.Language, s), r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeSize && r.Size == s:
				r.Size, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeRegion && r.Region == s:
				r.Region, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeContainer && r.Container == s:
				r.Container, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeGenre && r.Genre == s:
				r.Genre, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeGroup && r.Group == s:
				r.Group, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			case typ == TagTypeExt && r.Ext == s:
				r.Ext, r.tags[i] = "", r.tags[i].As(TagTypeText, nil)
			}
		} else if !movieSeriesEpisodeMusicGame && r.tags[i].Is(TagTypeSource) && ityp.Is(Movie, Series, Episode) {
			// reset movie/series/episode source tags
			if r.Source == r.tags[i].Normalize() {
				r.Source = ""
			}
			r.tags[i] = r.tags[i].As(TagTypeText, nil)
		}
	}
}

// titles sets the titles for the release.
func (b *TagBuilder) titles(r *Release) int {
	switch r.Type {
	case Movie:
		return b.movieTitles(r)
	case Series, Episode:
		return b.episodeTitles(r)
	case Music:
		return b.musicTitles(r)
	case Book, Audiobook:
		return b.bookTitles(r)
	}
	return b.defaultTitle(r)
}

// movieTitles sets the titles for movies and series.
func (b *TagBuilder) movieTitles(r *Release) int {
	var pos int
	// seek to text
	for ; pos < len(r.tags) && !r.tags[pos].Is(TagTypeText); pos++ {
	}
	start := pos
	var offset int
	r.Title, offset = b.title(r.tags[start:], TagTypeText)
	// seek date
	for pos = 0; pos < len(r.tags) && !r.tags[pos].Is(TagTypeDate); pos++ {
	}
	// check date position
	date := pos
	if date == len(r.tags) {
		return b.boxTitle(r, start, offset)
	}
	// seek resolution
	for pos = 0; pos < len(r.tags) && !r.tags[pos].Is(TagTypeResolution); pos++ {
	}
	resolution := pos
	if resolution == len(r.tags) {
		return b.boxTitle(r, start, offset)
	}
	// determine if all tags between date and resolution are either text, delimiter, cut, edition
	hasSubtitle := date+1 < resolution-1
	if !hasSubtitle {
		return b.boxTitle(r, start, offset)
	}
	for pos = date + 1; pos < len(r.tags) && hasSubtitle && pos < resolution; pos++ {
		hasSubtitle = hasSubtitle && r.tags[pos].Is(TagTypeDelim, TagTypeText, TagTypeCut, TagTypeEdition)
	}
	// capture subtitle
	if hasSubtitle {
		for pos = date + 1; pos < len(r.tags) && !r.tags[pos].Is(TagTypeText, TagTypeCut, TagTypeEdition); pos++ {
		}
		if pos < resolution-1 {
			r.Subtitle, _ = b.title(r.tags[pos:resolution-1], TagTypeText, TagTypeCut, TagTypeEdition)
		}
	}
	// alternate subtitle delimiter
	if i := strings.LastIndexByte(r.Title, '~'); i != -1 && r.Subtitle == "" {
		r.Title, r.Subtitle = strings.TrimRightFunc(r.Title[:i], isTitleTrimDelim), strings.TrimLeftFunc(r.Title[i+1:], isTitleTrimDelim)
	}
	return min(start+offset, resolution)
}

// boxTitle sets the titles for boxes.
func (b *TagBuilder) boxTitle(r *Release, start, offset int) int {
	n := start + offset
	if n == len(r.tags) || n <= 0 || r.Disc == "" || !r.tags[n].Is(TagTypeCut, TagTypeEdition) {
		return n
	}
	// check previous 4 words for 'the' and where NOT start of last text block
	for pos := n - 1; pos > start+1 && pos > n-8 && pos > 0; pos-- {
		if MustNormalize(r.tags[pos-1].Text()) == "the" {
			prefix, _ := b.title(r.tags[pos-1:n], TagTypeText)
			// consume remaining text, cut, edition
			suffix, offset := b.title(r.tags[n:], TagTypeText, TagTypeCut, TagTypeEdition)
			// reform title, subtitle
			r.Title = strings.TrimRightFunc(strings.TrimSuffix(r.Title, prefix), isTitleTrimDelim)
			r.Subtitle = prefix + " " + strings.TrimRightFunc(suffix, isBreakDelim)
			return n + offset
		}
	}
	return n
}

// episodeTitles sets the titles for episodes.
func (b *TagBuilder) episodeTitles(r *Release) int {
	// scan title
	pos := b.movieTitles(r)
	typ := TagTypeSeries
	if r.Month != 0 && r.Day != 0 {
		typ = TagTypeDate
	}
	// seek text after date/series, collecting any skipped text
	for ; pos < len(r.tags) && !r.tags[pos].Is(typ); pos++ {
		if r.tags[pos].Is(TagTypeText) {
			r.unused = append(r.unused, pos)
		}
	}
	if pos == len(r.tags) {
		return pos
	}
	// episode title must follow source, resolution, collection, date, series,
	// version, disc, other, cut, edition, language or container tags. it must
	// be before any codec, audio tag
	for pos++; pos < len(r.tags) && r.tags[pos].Is(
		TagTypeDelim,
		TagTypeSource,
		TagTypeResolution,
		TagTypeCollection,
		TagTypeDate,
		TagTypeSeries,
		TagTypeVersion,
		TagTypeDisc,
		TagTypeOther,
		TagTypeCut,
		TagTypeEdition,
		TagTypeLanguage,
		TagTypeContainer,
	); pos++ {
	}
	// check at text tag
	if pos == len(r.tags) || !r.tags[pos].Is(TagTypeText) {
		return pos
	}
	var offset int
	r.Subtitle, offset = b.title(r.tags[pos:], TagTypeText)
	return pos + offset
}

// musicTitles sets the titles for musics.
func (b *TagBuilder) musicTitles(r *Release) int {
	var i int
	r.Title, i = b.mixTitle(r, 0)
	// split artist, title
	for _, s := range []string{" - ", "--", "~", "-"} {
		if j := strings.LastIndex(r.Title, s); j != -1 {
			r.Artist, r.Title = strings.TrimRightFunc(r.Title[:j], isTitleTrimDelim), strings.TrimLeftFunc(r.Title[j+len(s):], isBreakDelim)
			break
		}
	}
	// check if at end / skipped date
	i, skipped, ok := b.checkDate(r, i)
	if ok {
		s := r.tags[i].Delim()
		// Artist - (Prefix) Title
		if r.Artist == "" && strings.HasSuffix(s, "(") {
			title, z := b.mixTitle(r, i+1)
			var subtitle string
			subtitle, z = b.mixTitle(r, z+1)
			if title != "" && subtitle != "" {
				r.Artist, r.Title = r.Title, "("+title+") "+subtitle
				if i, skipped, ok = b.checkDate(r, z); !ok {
					return i
				}
			}
		}
		// (Artist) - Title
		if r.Artist == "" && (skipped || strings.HasPrefix(s, ")")) {
			if title, z := b.mixTitle(r, i+1); title != "" {
				r.Artist, r.Title, i = r.Title, title, z
			}
		}
		// capture subtitle after '(', '__', '-', or '~'
		if r.Subtitle == "" &&
			(strings.HasSuffix(s, "(") || s == "__" || strings.ContainsAny(s, "-~")) &&
			peek(r.tags[:r.end], i+1, TagTypeText) {
			r.Subtitle, i = b.mixTitle(r, i+1)
		}
	}
	if r.Subtitle == "" && r.Artist != "" {
		// try to split artist
		for _, s := range []string{" - ", "--", "~"} {
			if j := strings.LastIndex(r.Artist, s); j != -1 {
				r.Artist, r.Title, r.Subtitle = strings.TrimRightFunc(r.Artist[:j], isTitleTrimDelim), strings.TrimLeftFunc(r.Artist[j+len(s):], isBreakDelim), r.Title
				break
			}
		}
	}
	return i
}

// mixTitle returns the mix title.
func (b *TagBuilder) mixTitle(r *Release, i int) (string, int) {
	start := min(b.start(r, i), len(r.tags))
	for i = start; i < r.end && r.tags[i].Is(TagTypeDelim, TagTypeText, TagTypeOther); i++ {
		if r.tags[i].Is(TagTypeOther) && r.tags[i].Other() != "REMiX" {
			break
		}
	}
	title, offset := b.title(r.tags[start:i], TagTypeText, TagTypeOther)
	return title, start + offset
}

// checkDate checks if i is at end of tags, and skips a date tag if present.
func (b *TagBuilder) checkDate(r *Release, i int) (int, bool, bool) {
	// bail if at end
	if i == r.end {
		return i, false, false
	}
	// skip a date in middle, such as Artist (2003) - Title
	var skipped bool
	if r.tags[i].Is(TagTypeDate) {
		i, skipped = i+1, true
	}
	// bail if not a delim
	if i == r.end || !r.tags[i].Is(TagTypeDelim) {
		return i, skipped, false
	}
	return i, skipped, true
}

// bookTitles sets the titles for books.
func (b *TagBuilder) bookTitles(r *Release) int {
	var s string
	var pos, offset int
	for ; pos < len(r.tags); pos += offset {
		// seek to text
		for ; pos < len(r.tags) && !r.tags[pos].Is(TagTypeText, TagTypePlatform, TagTypeArch, TagTypeOther, TagTypeRegion); pos++ {
		}
		if pos == len(r.tags) {
			break
		}
		switch isOther := r.tags[pos].Is(TagTypeOther); {
		case isOther && r.tags[pos].InfoType() != Book:
			offset = 1
			continue
		case isOther:
			s, offset = b.title(r.tags[pos:], TagTypeOther)
			s = strings.TrimFunc(s, isAnyDelim)
		default:
			s, offset = b.title(r.tags[pos:], TagTypeText, TagTypePlatform, TagTypeArch, TagTypeRegion)
		}
		if r.Title != "" && s != "" {
			r.Title += " "
		}
		r.Title += s
	}
	if i := strings.LastIndexByte(r.Title, ';'); i != -1 {
		r.Title, r.Subtitle = strings.TrimRightFunc(r.Title[:i], isTitleTrimDelim), strings.TrimLeftFunc(r.Title[i+1:], isTitleTrimDelim)
	}
	if r.Artist == "" {
		for _, s := range []string{" - ", "--", "~"} {
			if i := strings.Index(r.Title, s); i != -1 {
				r.Artist, r.Title = strings.TrimRightFunc(r.Title[:i], isTitleTrimDelim), strings.TrimLeftFunc(r.Title[i+len(s):], isBreakDelim)
				break
			}
		}
	}
	if r.Subtitle == "" {
		for _, s := range []string{" - ", "--", "~"} {
			if i := strings.Index(r.Title, s); i != -1 {
				r.Title, r.Subtitle = strings.TrimRightFunc(r.Title[:i], isBreakDelim), strings.TrimLeftFunc(r.Title[i+len(s):], isTitleTrimDelim)
				break
			}
		}
	}
	if i := strings.LastIndexByte(r.Title, '-'); r.Artist == "" && i != -1 {
		artist, title := strings.TrimRightFunc(r.Title[:i], isTitleTrimDelim), strings.TrimLeftFunc(r.Title[i+1:], isBreakDelim)
		if !b.digsuf.MatchString(artist) && !b.digpre.MatchString(title) {
			r.Artist, r.Title = artist, title
		}
	}
	return pos
}

// defaultTitle sets the default title.
func (b *TagBuilder) defaultTitle(r *Release) int {
	// seek to text
	var pos int
	for pos = 0; pos < len(r.tags) && !r.tags[pos].Is(TagTypeText); pos++ {
	}
	var offset int
	r.Title, offset = b.title(r.tags[pos:], TagTypeText)
	return pos + offset
}

// title finds all text in tags, stopping when encountering a non-space
// delimiter, returning the title and ending position.
func (b *TagBuilder) title(tags []Tag, types ...TagType) (string, int) {
	var v []string
	var i int
loop:
	for ; i < len(tags); i++ {
		switch {
		case tags[i].Is(types...):
			v = append(v, strings.ReplaceAll(tags[i].Text(), ".", " "))
		case tags[i].Is(TagTypeDelim):
			if s := tags[i].Delim(); !strings.ContainsAny(s, "()[]{}\\/") && s != "__" {
				v = append(v, b.delim(s, tags, i))
			} else {
				break loop
			}
		default:
			break loop
		}
	}
	// acronyms missing periods
	s := b.missing.ReplaceAllStringFunc(strings.Join(v, ""), func(a string) string {
		return strings.TrimLeft(strings.ReplaceAll(strings.TrimSpace(a), " ", "."), ". ") + ". "
	})
	// fix oopsie
	s = strings.ReplaceAll(s, ". .", ". ")
	// acronymns on single letters
	s = b.bad.ReplaceAllStringFunc(s, func(a string) string {
		return b.fix.ReplaceAllString(a, "$1")
	})
	// collapse spaces
	s = b.spaces.ReplaceAllString(s, " ")
	// collapse ellipsis
	s = b.ellips.ReplaceAllString(s, "...")
	// unescape entities
	s = html.UnescapeString(s)
	if m := b.plus.FindAllStringIndex(s, -1); len(m) > 1 {
		s = b.plus.ReplaceAllString(s, " ")
	}
	// trim
	return strings.TrimFunc(s, isTitleTrimDelim), i
}

// unused sets the unused text on the release.
func (b *TagBuilder) unused(r *Release, i int) {
	// collect
	for ; i < len(r.tags); i++ {
		if r.tags[i].Is(TagTypeText) {
			r.unused = append(r.unused, i)
		}
	}
	// final conversions
	if n := len(r.unused); n != 0 {
		switch s := r.tags[r.unused[n-1]].Text(); {
		case r.Sum == "" && b.sum.MatchString(s) && strings.ContainsAny(s, "0123456789"):
			r.Sum, r.unused = s, r.unused[:n-1]
		case r.Group == "" && !b.digits.MatchString(s):
			r.Group, r.unused = s, r.unused[:n-1]
		}
	}
}

// delim fixes the delimiter based on surrounding tags.
func (b *TagBuilder) delim(delim string, tags []Tag, i int) string {
	// special cases
	switch delim {
	case "...":
		return "..."
	case "..", ". ":
		return ". "
	case "":
		return " "
	}
	// convert to spaces, collapse
	s := b.spaces.ReplaceAllString(strings.Map(func(r rune) rune {
		switch r {
		case '-', '+', ',', '.', '~':
			return r
		case '\t', '\n', '\f', '\r', ' ', '_':
			return ' '
		}
		return -1
	}, delim), " ")
	// bail if last tag or not a period
	if s != "." || i == len(tags)-1 {
		return b.spaces.ReplaceAllString(strings.Map(func(r rune) rune {
			if r == '.' {
				return ' '
			}
			return r
		}, s), " ")
	}
	// get ante, prev, next
	var ante, prev, next string
	if i > 2 && tags[i-2].Is(TagTypeDelim) {
		ante = tags[i-2].Delim()
	}
	if i != 0 && tags[i-1].Is(TagTypeText) {
		prev = tags[i-1].Text()
	}
	if i < len(tags)-1 && tags[i+1].Is(TagTypeText) {
		next = tags[i+1].Text()
	}
	// acronyms / abbreviations
	if isUpperLetter([]rune(prev)) && isUpperLetter([]rune(next)) && !strings.ContainsAny(ante, "-~") {
		return "."
	}
	return " "
}

// isUpperLetter determines if s is all upper case.
func isUpperLetter(r []rune) bool {
	switch len(r) {
	case 0:
		return true
	case 1:
		return unicode.IsUpper(r[0])
	}
	return false
}

// contains returns if v contains str.
func contains(v []string, str string) bool {
	for _, s := range v {
		if str == s {
			return true
		}
	}
	return false
}

// remove removes str from v.
func remove(v []string, str string) []string {
	var s []string
	for _, t := range v {
		if str == t {
			continue
		}
		s = append(s, t)
	}
	return s
}

// min returns the min of a, b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// peek determines if i is of type.
func peek(tags []Tag, i int, types ...TagType) bool {
	return 0 <= i && i < len(tags) && tags[i].Is(types...)
}

// isolated determines if the tag in the indicated incremental direction
// (-1/+1) is not text/whitespace.
func isolated(tags []Tag, i, inc int) bool {
	for i += inc; 0 < i && i < len(tags)-1 && tags[i+inc].Is(TagTypeWhitespace, TagTypeDelim); i += inc {
	}
	i += inc
	return 0 <= i && i < len(tags)-1 && tags[i].Is(TagTypeText)
}
