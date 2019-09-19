package quantile

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgent(t *testing.T) {
	a := &Agent{}

	type testcase struct {
		// expected
		// s.Basic.Cnt should equal binsum + buf
		binsum int // expected sum(b.n) for bin in a.
		buf    int // expected len(a.buf)

		// action
		ninsert int  // ninsert values are inserted before checking
		flush   bool // flush before checking
		reset   bool // reset befor checking
	}

	setup := func(t *testing.T, tt testcase) {
		for i := 0; i < tt.ninsert; i++ {
			a.Insert(float64(i))
		}

		if tt.reset {
			a.Reset()
		}

		if tt.flush {
			a.flush()
		}
	}

	check := func(t *testing.T, exp testcase) {
		t.Helper()

		if l := len(a.Buf); l != exp.buf {
			t.Fatalf("len(a.buf) wrong. got:%d, want:%d", l, exp.buf)
		}

		binsum := 0
		for _, b := range a.Sketch.bins {
			binsum += int(b.n)
		}

		if got, want := binsum, exp.binsum; got != want {
			t.Fatalf("sum(b.n) wrong. got:%d, want:%d", got, want)
		}

		if got, want := a.Sketch.count, binsum; got != want {
			t.Fatalf("s.count should match binsum. got:%d, want:%d", got, want)
		}

		if got, want := int(a.Sketch.Basic.Cnt), exp.binsum+exp.buf; got != want {
			t.Fatalf("Summary.Cnt should equal len(buf)+s.count. got:%d, want: %d", got, want)
		}
	}

	// NOTE: these tests share the same sketch, so every test depends on the
	// previous test.
	for _, tt := range []testcase{
		{binsum: 0, buf: agentBufCap - 1, ninsert: agentBufCap - 1},
		{binsum: agentBufCap, buf: 0, ninsert: 1},
		{binsum: agentBufCap, buf: 1, ninsert: 1},
		{binsum: 2 * agentBufCap, buf: 1, ninsert: agentBufCap},
		{binsum: 2*agentBufCap + 1, buf: 0, flush: true},
		{reset: true},
		{flush: true},
	} {
		setup(t, tt)
		check(t, tt)
	}
}

func TestAgentFinish(t *testing.T) {
	t.Run("DeepCopy", func(t *testing.T) {
		var (
			binsptr = func(s *Sketch) uintptr {
				hdr := (*reflect.SliceHeader)(unsafe.Pointer(&s.bins))
				return hdr.Data
			}

			checkDeepCopy = func(a *Agent, s *Sketch) {
				if binsptr(&a.Sketch) == binsptr(s) {
					t.Fatal("finished sketch should not share the same bin array")
				}

				if !a.Sketch.Equals(s) {
					t.Fatal("sketches should be equal")
				}
				require.Equal(t, a.Sketch, *s)
			}

			aSketch = &Agent{}
		)

		aSketch.Insert(1)
		finished := aSketch.Finish()
		checkDeepCopy(aSketch, finished)
	})

	t.Run("Empty", func(t *testing.T) {
		a := &Agent{}
		require.Nil(t, a.Finish())
	})
}

func TestInsertBucket(t *testing.T) {
	type testcase struct {
		// The bucket insertion parameters
		low, high float64
		count     uint
		// The expected bins
		bins binList
	}

	for _, tt := range []testcase{
		{1, 2, 10, binList{ // Basic case
			{1341, 1},
			{1347, 1},
			{1352, 1},
			{1357, 1},
			{1361, 1},
			{1366, 1},
			{1370, 1},
			{1374, 1},
			{1377, 1},
			{1381, 1}}},
		{129, 130, 10, binList{{1650, 10}}}, // All of the points go into one bucket
	} {
		a := &Agent{}

		a.InsertBucket(tt.low, tt.high, tt.count)
		assert.Equal(t, tt.bins, a.Sketch.bins)
	}
}
