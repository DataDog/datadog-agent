package quantile

import (
	"reflect"
	"testing"
	"unsafe"

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

func TestAgentInterpolation(t *testing.T) {
	a := &Agent{}

	type testcase struct {
		// expected
		// s.Basic.Cnt should equal binsum + buf
		lower float64 // lower bound for interpolation
		upper float64 // upper bound for interpolation
		count uint    //  values are inserted before checking

		exp string
		e   float64 // acceptable error
	}

	check := func(t *testing.T, exp *Sketch, e float64) {
		t.Helper()

		if !a.Sketch.Equals(exp) {
			t.Errorf("sketches should be equal\nactual %s\nexp %s", a.Sketch.String(), exp.String())
			t.Fail()
		}
	}

	// NOTE: these tests share the same sketch, so every test depends on the
	// previous test.
	for _, tt := range []testcase{
		{lower: 0, upper: 10, count: 2, exp: "0:1 1442:1"}, // sparse,
		// dense, even
		{lower: 0, upper: 10, count: 100, exp: "0:2 1190:1 1235:1 1261:1 1280:1 1295:1 1307:1 1317:1 1326:1 1334:1 1341:1 1347:1 1353:1 1358:1 1363:1 1368:1 1372:2 1376:1 1380:1 1384:1 1388:1 1391:1 1394:1 1397:2 1400:1 1403:1 1406:2 1409:1 1412:1 1415:2 1417:1 1419:1 1421:1 1423:1 1425:1 1427:1 1429:2 1431:1 1433:1 1435:2 1437:1 1439:2 1441:1 1442:1 1443:2 1445:2 1447:1 1449:2 1451:2 1453:2 1455:2 1457:2 1459:1 1460:1 1461:1 1462:1 1463:1 1464:1 1465:1 1466:1 1467:2 1468:1 1469:1 1470:1 1471:1 1472:2 1473:1 1474:1 1475:1 1476:2 1477:1 1478:2 1479:1 1480:1 1481:2 1482:1 1483:2 1484:1 1485:2 1486:1"},
	} {
		a.InsertInterpolate(tt.lower, tt.upper, tt.count)
		exp := ParseSketch(t, tt.exp)
		check(t, exp, tt.e)
	}
}
