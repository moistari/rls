// Package reutil has regexp util funcs.
package reutil

import (
	"strings"

	"github.com/moistari/rls/taginfo"
)

// Join joins strings into a regexp (ie, `(str 0|str 1|...)`), optionally
// quoting each string with \Q\E.
func Join(quote bool, strs ...string) string {
	// copy strings and quote
	v := make([]string, len(strs))
	if !quote {
		copy(v, strs)
	} else {
		for i := 0; i < len(v); i++ {
			v[i] = `\Q` + strs[i] + `\E`
		}
	}
	return strings.Join(v, `|`)
}

// Build builds a regexp for strings.
//
// Config options:
//	i - ignore case
//	^ - add ^ start anchor
//	a - add \b start anchor
//	q - quote each string with \Q\E
//	b - add \b end anchor
//	$ - add $ end anchor
func Build(config string, strs ...string) string {
	var s []string
	// ignore case
	if strings.Contains(config, `i`) {
		s = append(s, `(?i)`)
	}
	// start
	if strings.Contains(config, `^`) {
		s = append(s, `^`)
	}
	if strings.Contains(config, `a`) {
		s = append(s, `\b`)
	}
	s = append(s, `(`, Join(strings.Contains(config, "q"), strs...), `)`)
	// end
	if strings.Contains(config, `b`) {
		s = append(s, `\b`)
	}
	if strings.Contains(config, `$`) {
		s = append(s, `$`)
	}
	return strings.Join(s, ``)
}

// Taginfo builds a regexp for tag info.
//
// See Build for confg options.
func Taginfo(config string, infos ...*taginfo.Taginfo) string {
	v := make([]string, len(infos))
	for i, info := range infos {
		v[i] = info.RE()
	}
	return Build(config, v...)
}
