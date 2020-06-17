package quantile

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// ParseBuf creates a slice of float64s from the given dsl
// TODO|DOC: add more examples
func ParseBuf(t *testing.T, dsl string) []float64 {
	t.Helper()
	var (
		c   = Default()
		out []float64
	)

	eachParsedToken(t, dsl, 16, func(k Key, n uint64) {
		if n > maxBinWidth {
			t.Fatal("n > max", n, maxBinWidth)
		}

		for i := uint64(0); i < n; i++ {
			out = append(out, c.f64(k))
		}
	})

	return out
}

// ParseSketch creates a sketch with the exact bin layout given in the dsl
// TODO|DOC: add more examples
//
// NOTE: there is no guarantee that the sketch is correct (because we'd like to
// test bad sketch handling)
func ParseSketch(t *testing.T, dsl string) *Sketch {
	t.Helper()
	s := &Sketch{}
	c := Default()

	eachParsedToken(t, dsl, 16, func(k Key, n uint64) {
		if n > maxBinWidth {
			t.Fatal("n > max", n, maxBinWidth)
		}

		s.count += int(n)
		s.bins = append(s.bins, bin{k: k, n: uint16(n)})
		s.Basic.InsertN(c.f64(k), float64(n))
	})

	return s
}

func parseKey(t *testing.T, s string) Key {
	t.Helper()
	k, err := strconv.ParseInt(s, 10, 16)
	if err != nil {
		t.Fatal("bad incr key:", s)
	}

	return Key(k)
}

func parseN(t *testing.T, s string) uint64 {
	t.Helper()
	if !strings.HasPrefix(s, "max") {
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			t.Fatal("bad incr n:", s)
		}

		return n
	}

	// <k>:max case
	n := uint64(math.MaxUint16)
	switch {
	case s == "max":
		return n
	case len(s) <= 4:
		t.Fatal("invalid max token:", s)
	}

	modifier, err := strconv.ParseUint(s[4:], 10, 64)
	if err != nil {
		t.Fatal("bad -modifier n:", s)
	}

	// special max modifiers
	switch s[3] {
	case '-':
		n -= modifier
	case '*':
		n *= modifier
	case '/':
		n /= modifier
	}

	return n
}

// eachParsedToken parses a bin dsl "<key>:<n> <key>:<n>" and calls f for each token
// examples:
// - "0:max"   = Bin{k:0, n:maxBinWidth}
// - "1:2"     = Bin{k:1, n:2}
// - "3:max-2" = Bin{k:3, n:maxBinWidth-2}
func eachParsedToken(t *testing.T, dsl string, bitSize uint, f func(Key, uint64)) {
	t.Helper()
	if dsl == "" {
		return
	}

	for _, tok := range strings.Split(dsl, " ") {
		i := strings.IndexByte(tok, ':')
		if 0 > i {
			t.Fatal("bad incr tpl:", tok)
		}
		k := parseKey(t, tok[:i])
		n := parseN(t, tok[i+1:])

		// make sure this value can be cast to our bitSize
		if maxn := uint64(1)<<bitSize - 1; n > maxn {
			t.Errorf("n must be less than max. n=%d max=%d bitSize=%d",
				n, maxn, bitSize)
		}

		f(k, n)
	}
}

// arange is like np.arange, except it creates a sketch
// inserting ([start,] stop[, step,]) values.
func arange(t *testing.T, c *Config, args ...int) *Sketch {
	t.Helper()
	var (
		start, stop int
		step        = 1
	)

	switch len(args) {
	case 1:
		stop = args[0]
	case 2:
		start, stop = args[0], args[1]
	case 3:
		start, stop, step = args[0], args[1], args[2]
	default:
		t.Fatalf("too many args: %v", args)
	}

	s := &Sketch{}
	for i := start; i < stop; i += step {
		s.Insert(c, float64(i))
	}

	return s
}
