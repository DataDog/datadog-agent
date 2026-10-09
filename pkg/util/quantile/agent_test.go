// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"math"
	"math/bits"
	"runtime"
	"testing"
	"time"
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

		sampleRate float64 // apply this rate to the sample
	}

	setup := func(tt testcase) {
		for i := 0; i < tt.ninsert; i++ {
			a.Insert(float64(i), tt.sampleRate)
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

		if got, want := a.Sketch.count, uint64(binsum); got != want {
			t.Fatalf("s.count should match binsum. got:%d, want:%d", got, want)
		}

		if got, want := int(a.Sketch.Basic.Cnt), exp.binsum+exp.buf; got != want {
			t.Fatalf("Summary.Cnt should equal len(buf)+s.count. got:%d, want: %d", got, want)
		}
	}

	// NOTE: these tests share the same sketch, so every test depends on the
	// previous test.
	for _, tt := range []testcase{
		{binsum: 0, buf: agentBufCap - 1, ninsert: agentBufCap - 1, sampleRate: 1},
		{binsum: agentBufCap, buf: 0, ninsert: 1, sampleRate: 1},
		{binsum: agentBufCap, buf: 1, ninsert: 1, sampleRate: 1},
		{binsum: 2 * agentBufCap, buf: 1, ninsert: agentBufCap, sampleRate: 1},
		{binsum: 2*agentBufCap + 1, buf: 0, flush: true, sampleRate: 1},
		{reset: true, sampleRate: 1},
		{flush: true, sampleRate: 1},
		{binsum: 20, ninsert: 2, flush: true, sampleRate: .1},
		{binsum: 22, ninsert: 2, flush: true, sampleRate: 1},
	} {
		setup(tt)
		check(t, tt)
	}
}

func TestAgentFinish(t *testing.T) {
	t.Run("DeepCopy", func(t *testing.T) {
		var (
			binsptr = func(s *Sketch) uintptr {
				hdr := unsafe.SliceData(s.bins)
				return uintptr(unsafe.Pointer(hdr))
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

		aSketch.Insert(1, 1)
		finished := aSketch.Finish()
		checkDeepCopy(aSketch, finished)
	})

	t.Run("Empty", func(t *testing.T) {
		a := &Agent{}
		require.Nil(t, a.Finish())
	})
}

// TestAgentNaNSampleRate guards against a NaN sample rate corrupting the sketch
// count. See SMPTNG-761.
func TestAgentNaNSampleRate(t *testing.T) {
	a := &Agent{}
	a.Insert(1, math.NaN())
	a.Insert(1, math.NaN())

	s := a.Finish()
	require.NotNil(t, s, "an even count of NaN-sample-rate inserts must not yield a nil sketch")
	require.EqualValues(t, 2, s.Basic.Cnt, "two samples must count as two observations")
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

	check := func(t *testing.T, tt testcase) {
		t.Helper()

		exp := ParseSketch(t, tt.exp)

		if tt.count != uint(exp.Basic.Cnt) {
			t.Errorf("Expected sketch has wrong count %v (expected %v)", exp.Basic.Cnt, tt.count)
			t.Fail()
		}

		if tt.count != uint(a.Sketch.Basic.Cnt) {
			t.Errorf("Actual sketch has wrong count %v (expected %v)", a.Sketch.Basic.Cnt, tt.count)
			t.Fail()
		}

		if !a.Sketch.ApproxEquals(exp, tt.e) {
			t.Errorf("sketches should be equal\nactual %s\nexp %s", a.Sketch.String(), exp.String())
			t.Fail()
		}

		var actCount uint
		for _, b := range a.Sketch.bins {
			actCount += uint(b.n)
		}

		if tt.count != actCount {
			t.Errorf("Actual sketch bins total wrong count %v (expected %v)", actCount, tt.count)
			t.Fail()
		}
	}

	for _, tt := range []testcase{
		{lower: 0, upper: 10, count: 2, exp: "0:1 1442:1"},                       // sparse,
		{lower: 10, upper: 20, count: 4, exp: "1487:1 1502:1 1514:1 1524:1"},     // sparse,
		{lower: -10, upper: 10, count: 4, exp: "-1487:1 -1442:1 -1067:1 1442:1"}, // negative,
		// dense, even
		{lower: 0, upper: 10, count: 100, exp: "0:1 1190:1 1235:1 1261:1 1280:1 1295:1 1307:1 1317:1 1326:1 1334:1 1341:1 1347:1 1353:1 1358:1 1363:1 1368:1 1372:2 1376:1 1380:1 1384:1 1388:1 1391:1 1394:1 1397:2 1400:1 1403:1 1406:2 1409:1 1412:1 1415:2 1417:1 1419:1 1421:1 1423:1 1425:1 1427:1 1429:2 1431:1 1433:1 1435:2 1437:1 1439:2 1441:1 1443:2 1445:2 1447:1 1449:2 1451:2 1453:2 1455:2 1457:2 1459:1 1460:1 1461:1 1462:1 1463:1 1464:1 1465:1 1466:1 1467:2 1468:1 1469:1 1470:1 1471:1 1472:2 1473:1 1474:1 1475:1 1476:2 1477:1 1478:2 1479:1 1480:1 1481:2 1482:1 1483:2 1484:1 1485:2 1486:1"},
		//large, dense, odd
		{lower: 1e3, upper: 1e5, count: 1e6 - 1, exp: "1784:158 1785:162 1786:164 1787:166 1788:170 1789:171 1790:175 1791:177 1792:180 1793:183 1794:185 1795:189 1796:191 1797:195 1798:197 1799:201 1800:203 1801:207 1802:210 1803:214 1804:217 1805:220 1806:223 1807:227 1808:231 1809:234 1810:238 1811:242 1812:245 1813:249 1814:253 1815:257 1816:261 1817:265 1818:270 1819:273 1820:278 1821:282 1822:287 1823:291 1824:295 1825:300 1826:305 1827:310 1828:314 1829:320 1830:324 1831:329 1832:335 1833:340 1834:345 1835:350 1836:356 1837:362 1838:367 1839:373 1840:379 1841:384 1842:391 1843:397 1844:403 1845:409 1846:416 1847:422 1848:429 1849:435 1850:442 1851:449 1852:457 1853:463 1854:470 1855:478 1856:486 1857:493 1858:500 1859:509 1860:516 1861:525 1862:532 1863:541 1864:550 1865:558 1866:567 1867:575 1868:585 1869:594 1870:603 1871:612 1872:622 1873:632 1874:642 1875:651 1876:662 1877:672 1878:683 1879:693 1880:704 1881:716 1882:726 1883:738 1884:749 1885:761 1886:773 1887:785 1888:797 1889:809 1890:823 1891:835 1892:848 1893:861 1894:875 1895:889 1896:902 1897:917 1898:931 1899:945 1900:960 1901:975 1902:991 1903:1006 1904:1021 1905:1038 1906:1053 1907:1071 1908:1087 1909:1104 1910:1121 1911:1138 1912:1157 1913:1175 1914:1192 1915:1212 1916:1231 1917:1249 1918:1269 1919:1290 1920:1309 1921:1329 1922:1351 1923:1371 1924:1393 1925:1415 1926:1437 1927:1459 1928:1482 1929:1506 1930:1529 1931:1552 1932:1577 1933:1602 1934:1626 1935:1652 1936:1678 1937:1704 1938:1731 1939:1758 1940:1785 1941:1813 1942:1841 1943:1870 1944:1900 1945:1929 1946:1959 1947:1990 1948:2021 1949:2052 1950:2085 1951:2117 1952:2150 1953:2184 1954:2218 1955:2253 1956:2287 1957:2324 1958:2360 1959:2396 1960:2435 1961:2472 1962:2511 1963:2550 1964:2589 1965:2631 1966:2671 1967:2714 1968:2755 1969:2799 1970:2842 1971:2887 1972:2932 1973:2978 1974:3024 1975:3071 1976:3120 1977:3168 1978:3218 1979:3268 1980:3319 1981:3371 1982:3423 1983:3477 1984:3532 1985:3586 1986:3643 1987:3700 1988:3757 1989:3816 1990:3876 1991:3936 1992:3998 1993:4060 1994:4124 1995:4188 1996:4253 1997:4320 1998:4388 1999:4456 2000:4526 2001:4596 2002:4668 2003:4741 2004:4816 2005:4890 2006:4967 2007:5044 2008:5124 2009:5203 2010:5285 2011:5367 2012:5451 2013:5536 2014:5623 2015:5711 2016:5800 2017:5890 2018:5983 2019:6076 2020:6171 2021:6267 2022:6365 2023:6465 2024:6566 2025:6668 2026:6773 2027:6878 2028:6986 2029:7095 2030:7206 2031:7318 2032:7433 2033:7549 2034:7667 2035:7786 2036:7909 2037:8032 2038:8157 2039:8285 2040:8414 2041:8546 2042:8679 2043:8815 2044:8953 2045:9092 2046:9235 2047:9379 2048:9525 2049:9675 2050:9825 2051:9979 2052:10135 2053:10293 2054:10454 2055:10618 2056:10783 2057:10952 2058:11123 2059:11297 2060:11473 2061:11653 2062:11834 2063:12020 2064:12207 2065:12398 2066:12592 2067:12788 2068:12989 2069:13191 2070:13397 2071:13607 2072:13819 2073:14036 2074:14254 2075:14478 2076:14703 2077:14933 2078:15167 2079:15403 2080:8942"},
		{lower: 1e3, upper: 1e4, count: 1e7 - 1, e: 1e-11, exp: "1784:17485 1785:17758 1786:18035 1787:18318 1788:18604 1789:18894 1790:19190 1791:19489 1792:19794 1793:20103 1794:20418 1795:20736 1796:21061 1797:21389 1798:21724 1799:22063 1800:22408 1801:22758 1802:23113 1803:23475 1804:23841 1805:24215 1806:24592 1807:24977 1808:25366 1809:25764 1810:26165 1811:26575 1812:26990 1813:27412 1814:27839 1815:28275 1816:28717 1817:29165 1818:29622 1819:30083 1820:30554 1821:31032 1822:31516 1823:32009 1824:32509 1825:33016 1826:33533 1827:34057 1828:34589 1829:35129 1830:35678 1831:36235 1832:36802 1833:37377 1834:37961 1835:38554 1836:39156 1837:39768 1838:40390 1839:41020 1840:41662 1841:42312 1842:42974 1843:43645 1844:44327 1845:45020 1846:45723 1847:46438 1848:47163 1849:47900 1850:48648 1851:49409 1852:50181 1853:50964 1854:51761 1855:52570 1856:53391 1857:54226 1858:55072 1859:55934 1860:56807 1861:57695 1862:58596 1863:59512 1864:60441 1865:61387 1866:62345 1867:63319 1868:64309 1869:65314 1870:66334 1871:67370 1872:68424 1873:69492 1874:70578 1875:71681 1876:72801 1877:73939 1878:75094 1879:76267 1880:77458 1881:78670 1882:79898 1883:81147 1884:82414 1885:83703 1886:85010 1887:86338 1888:87688 1889:89058 1890:90449 1891:91862 1892:93298 1893:94756 1894:96236 1895:97740 1896:99267 1897:100818 1898:102393 1899:103993 1900:105619 1901:107268 1902:108944 1903:110647 1904:112376 1905:114131 1906:115915 1907:117726 1908:119565 1909:121434 1910:123331 1911:125258 1912:127215 1913:129203 1914:131222 1915:133272 1916:135355 1917:137469 1918:139617 1919:141799 1920:144015 1921:146265 1922:148550 1923:150871 1924:153228 1925:155623 1926:158055 1927:160523 1928:163033 1929:165579 1930:168167 1931:170794 1932:17411"},
	} {
		a.Reset()
		a.InsertInterpolate(tt.lower, tt.upper, tt.count)
		check(t, tt)
	}

	// test double insert for adding overflows
	for _, tt := range []testcase{
		{lower: 1e3, upper: 1e4, count: 1e7 - 1, e: 2e-4, exp: "1784:34970 1785:35516 1786:36070 1787:36636 1788:37208 1789:37788 1790:38380 1791:38978 1792:39588 1793:40206 1794:40836 1795:41472 1796:42122 1797:42778 1798:43448 1799:44126 1800:44816 1801:45516 1802:46226 1803:46950 1804:47682 1805:48430 1806:49184 1807:49954 1808:50732 1809:51528 1810:52330 1811:53150 1812:53980 1813:54824 1814:55678 1815:56550 1816:57434 1817:58330 1818:59244 1819:60166 1820:61108 1821:62064 1822:63032 1823:64018 1824:65018 1825:66032 1826:67066 1827:68114 1828:69178 1829:70258 1830:71356 1831:72470 1832:73604 1833:74754 1834:75922 1835:77108 1836:78312 1837:79536 1838:80780 1839:82040 1840:83324 1841:84624 1842:85948 1843:87290 1844:88654 1845:90040 1846:91446 1847:92876 1848:94326 1849:95800 1850:97296 1851:98818 1852:100362 1853:101928 1854:103522 1855:105140 1856:106782 1857:108452 1858:110144 1859:111868 1860:113614 1861:115390 1862:117192 1863:119024 1864:120882 1865:122774 1866:124690 1867:126638 1868:128618 1869:130628 1870:132668 1871:134740 1872:136848 1873:138984 1874:141156 1875:143362 1876:145602 1877:147878 1878:150188 1879:152534 1880:154916 1881:157340 1882:159796 1883:162294 1884:164828 1885:167406 1886:170020 1887:172676 1888:175376 1889:178116 1890:180898 1891:183724 1892:186596 1893:189512 1894:192472 1895:195480 1896:198534 1897:201636 1898:204786 1899:207986 1900:211238 1901:214536 1902:217888 1903:221294 1904:224752 1905:228262 1906:231830 1907:235452 1908:239130 1909:242868 1910:246662 1911:250516 1912:254430 1913:258406 1914:262444 1915:266544 1916:270710 1917:274938 1918:279234 1919:283598 1920:288030 1921:292530 1922:297100 1923:301742 1924:306456 1925:311246 1926:316110 1927:321046 1928:326066 1929:331158 1930:336334 1931:341588 1932:34822"},
	} {
		a.Reset()
		a.InsertInterpolate(tt.lower, tt.upper, tt.count)
		a.InsertInterpolate(tt.lower, tt.upper, tt.count)
		tt.count = tt.count * 2
		check(t, tt)
	}
}

// TestAgentInterpolationBoundedKeys exercises the bounds enforced by
// InsertInterpolate. Before bounds were enforced, an upper bound large enough
// to saturate key() at uvinf made the loop counter (int16 Key) overflow when
// incremented past 32767, producing an unbounded slice and an effectively
// infinite loop. Each case must complete in well under the watchdog budget.
func TestAgentInterpolationBoundedKeys(t *testing.T) {
	// 1e300 is finite but large enough that agentConfig.key(±1e300) saturates
	// at ±uvinf. The pre-fix code would loop forever on these inputs.
	const (
		bigPos = 1e300
		bigNeg = -1e300
	)

	cases := []struct {
		name    string
		lower   float64
		upper   float64
		count   uint
		wantErr string // empty = no error expected
		wantCnt int64  // only checked when wantErr is empty
	}{
		{name: "saturating upper bound", lower: 1, upper: bigPos, count: 10, wantCnt: 10},
		{name: "saturating both bounds", lower: bigNeg, upper: bigPos, count: 100, wantCnt: 100},
		{name: "saturating lower bound only", lower: bigNeg, upper: 1, count: 50, wantCnt: 50},
		// Both bounds finite but past the sketch's representable range: key(lower)
		// and key(upper) both saturate to uvinf. Pre-clamp, binLow(uvinf) returned
		// +Inf and poisoned Sketch.Basic.
		{name: "both bounds saturate positive", lower: 1e300, upper: 1e301, count: 10, wantCnt: 10},
		{name: "both bounds saturate negative", lower: -1e301, upper: -1e300, count: 10, wantCnt: 10},
		{name: "non-monotonic bounds", lower: 100, upper: 1, count: 5, wantErr: ErrNonMonotonicBoundaries},
		{name: "equal bounds", lower: 42, upper: 42, count: 7, wantCnt: 7},
		{name: "zero count", lower: 1, upper: 100, count: 0, wantCnt: 0},
	}

	// Generous budget; a correctly bounded call iterates at most ~65535 keys.
	const budget = 5 * time.Second

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			a := &Agent{}

			done := make(chan error, 1)
			go func() {
				done <- a.InsertInterpolate(tt.lower, tt.upper, tt.count)
			}()

			select {
			case err := <-done:
				if tt.wantErr != "" {
					require.EqualError(t, err, tt.wantErr)
					return
				}
				require.NoError(t, err)
				require.Equal(t, tt.wantCnt, a.Sketch.Basic.Cnt)
				if tt.count > 0 {
					// Sketch.Basic stats must stay finite even when the input
					// bounds saturate key() to ±uvinf.
					b := a.Sketch.Basic
					require.False(t, math.IsInf(b.Min, 0), "Min must be finite, got %v", b.Min)
					require.False(t, math.IsInf(b.Max, 0), "Max must be finite, got %v", b.Max)
					require.False(t, math.IsInf(b.Sum, 0), "Sum must be finite, got %v", b.Sum)
					require.False(t, math.IsInf(b.Avg, 0), "Avg must be finite, got %v", b.Avg)
				}
			case <-time.After(budget):
				t.Fatalf("InsertInterpolate(%v, %v, %d) did not complete within %v",
					tt.lower, tt.upper, tt.count, budget)
			}
		})
	}
}

// TestAgentInsertSampleRateFitsInABin covers the dogstatsd side: a @rate becomes
// a count of 1/sampleRate, which used to take a uint16 bin per 65535 of it.
func TestAgentInsertSampleRateFitsInABin(t *testing.T) {
	// A variable, not a constant: Insert truncates 1/sampleRate in float64, giving
	// 999999999 here, which exact constant arithmetic would hide.
	sampleRate := 1e-9

	a := &Agent{}
	a.Insert(1, sampleRate)

	require.Equal(t, int64(1/sampleRate), a.Sketch.Basic.Cnt)
	require.Len(t, a.Sketch.bins, 1)
	require.Equal(t, uint64(a.Sketch.Basic.Cnt), a.Sketch.count)
}

// TestAgentInsertInterpolateHugeCountBoundedMemory inserts a single bucket holding
// far more than Config.MaxCount() observations, like the delta of the GPU NVLink
// FEC histogram from the NVML mock. Neither what the insert allocates nor what the
// sketch keeps may grow with the count, and the count and quantiles must survive.
func TestAgentInsertInterpolateHugeCountBoundedMemory(t *testing.T) {
	if bits.UintSize < 64 {
		t.Skip("the count does not fit in a 32-bit uint")
	}

	const (
		// One bin list at the default binLimit takes 16 KiB. Holding the count at
		// one bin per 65535 observations would take ~83 MiB instead.
		budget = 1 << 20
	)
	// A variable, so that the uint conversion below compiles on 32-bit targets.
	var count uint64 = 1_420_000_000_000

	a := &Agent{}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	err := a.InsertInterpolate(0, 0, uint(count))
	runtime.ReadMemStats(&after)
	require.NoError(t, err)

	require.LessOrEqual(t, after.TotalAlloc-before.TotalAlloc, uint64(budget),
		"bytes allocated by the insert")
	_, retained := a.Sketch.MemSize()
	require.LessOrEqual(t, retained, budget, "bytes retained by the sketch")

	require.Equal(t, int64(count), a.Sketch.Basic.Cnt)
	_, n := a.Sketch.Cols()
	var binTotal uint64
	for _, v := range n {
		binTotal += uint64(v)
	}
	require.InEpsilon(t, count, binTotal, 1e-6, "observations reported in the bins")

	for _, q := range []float64{0.01, 0.5, 0.99} {
		require.Zero(t, a.Sketch.Quantile(Default(), q), "quantile %v", q)
	}
}

// TestAgentLowSampleRateQuantiles inserts 500 values, over 3.4 decades, at a
// DogStatsD sample rate of 1e-6. With uint16 bins each value took 16 bins, so the
// sketch went past binLimit, and trimLeft moved the lowest observations into
// higher bins: the 1st percentile was 25% too high. Every quantile must stay
// within the error of a bin.
func TestAgentLowSampleRateQuantiles(t *testing.T) {
	const values = 500
	c := Default()

	a := &Agent{}
	vals := make([]float64, values)
	for i := range vals {
		vals[i] = math.Pow(1.016, float64(i))
		a.Insert(vals[i], 1e-6)
	}
	s := a.Finish()

	for _, q := range []float64{0.01, 0.1, 0.25, 0.5, 0.75, 0.9, 0.99} {
		want := vals[int(math.Round(q*(values-1)))]
		require.InEpsilon(t, want, s.Quantile(c, q), 0.025, "quantile %v", q)
	}
	require.Equal(t, values, len(s.bins), "one bin per value")
}
