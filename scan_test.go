package rls

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestScanner_library(t *testing.T) {
	if s := os.Getenv("TESTS"); s != "library" {
		return
	}
	f, err := os.OpenFile("library.txt", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scan(t, newBaseScanner(f))
}

func TestScanner_releaselist(t *testing.T) {
	if s := os.Getenv("TESTS"); s != "releaselist" {
		return
	}
	f, err := os.OpenFile("releaselist.txt", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scan(t, bufio.NewScanner(f) /*, WithWorkers(1)*/)
}

func scan(t *testing.T, scanner Scanner, opts ...ReleaseScannerOption) {
	start, prev, i := time.Now(), time.Now(), 0
	progress := func(typ string) {
		n := time.Now()
		if i != 0 {
			d := n.Sub(start)
			avg := d / time.Duration(i)
			t.Logf("%d: %s (%s) RUNTIME: %s DELTA: %s AVG: %v",
				i, typ, n.Format(time.RFC3339), d.Truncate(time.Millisecond), n.Sub(prev).Truncate(time.Millisecond), avg)
		}
		prev = n
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	unused := struct {
		count map[string]uint64
		items map[string][]string
	}{
		count: make(map[string]uint64),
		items: make(map[string][]string),
	}
	m, s := make(map[string]uint64), NewScanner(opts...)
loop:
	for ch := s.Scan(ctx, scanner); ; i++ {
		if i != 0 && i%10000 == 0 {
			progress("PROGRESS")
		}
		select {
		case <-ctx.Done():
			if err := ctx.Err(); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		case v := <-ch:
			if v == nil || v.ID == 0 {
				break loop
			}
			if id, ok := m[v.Line]; ok {
				t.Errorf("%d: %d: DUPLICATE %q (%d)", i, v.ID, string(v.Line), id)
				continue
			}
			if u := v.Release.Unused(); len(u) != 0 {
				for _, tag := range u {
					if s := tag.Text(); !num.MatchString(s) {
						unused.count[s]++
						unused.items[s] = append(unused.items[s], v.Line)
					}
				}
				t.Logf("%d: %d: UNUSED: %q - %q - %s", i, v.ID, string(v.Line), v.Release.Type, joinTags(u, "%s", " "))
			}
		}
	}
	progress("DONE")
	keys := make([]kv, len(unused.count))
	for k := range unused.count {
		keys = append(keys, kv{
			k: k,
			v: unused.count[k],
		})
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].v > keys[j].v
	})
	for i := 0; i < 2000 && i < len(keys); i++ {
		t.Logf("TOP %02d: %q %d", 2000-i, keys[i].k, keys[i].v)
		for j := uint64(0); j < 10 && j < keys[i].v; j++ {
			t.Logf("    %02d: % 2d: %q", 2000-i, j, unused.items[keys[i].k][j])
		}
	}
}

type unusedCount struct {
	items []string
	count int64
}

var num = regexp.MustCompile(`^\d+$`)

type kv struct {
	k string
	v uint64
}

type baseScanner struct {
	s *bufio.Scanner
}

func newBaseScanner(r io.Reader) *baseScanner {
	return &baseScanner{
		s: bufio.NewScanner(r),
	}
}

func (s *baseScanner) Scan() bool {
	return s.s.Scan()
}

func (s *baseScanner) Text() string {
	return filepath.Base(strings.TrimSuffix(s.s.Text(), "\n")) + "\n"
}

func (s *baseScanner) Err() error {
	return s.s.Err()
}
