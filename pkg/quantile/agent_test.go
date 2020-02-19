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

		if !a.Sketch.Equals(exp) {
			t.Errorf("sketches should be equal\nactual %s\nexp %s", a.Sketch.String(), exp.String())
			t.Fail()
		}
	}

	// NOTE: these tests share the same sketch, so every test depends on the
	// previous test.
	for _, tt := range []testcase{
		{lower: 0, upper: 10, count: 2, exp: "0:1 1442:1"},                       // sparse,
		{lower: 10, upper: 20, count: 4, exp: "1487:1 1502:1 1514:1 1524:1"},     // sparse,
		{lower: -10, upper: 10, count: 4, exp: "-1487:1 -1442:1 -1067:1 1442:1"}, // negative,
		// dense, even
		{lower: 0, upper: 10, count: 100, exp: "0:1 1190:1 1235:1 1261:1 1280:1 1295:1 1307:1 1317:1 1326:1 1334:1 1341:1 1347:1 1353:1 1358:1 1363:1 1368:1 1372:2 1376:1 1380:1 1384:1 1388:1 1391:1 1394:1 1397:2 1400:1 1403:1 1406:2 1409:1 1412:1 1415:2 1417:1 1419:1 1421:1 1423:1 1425:1 1427:1 1429:2 1431:1 1433:1 1435:2 1437:1 1439:2 1441:1 1443:2 1445:2 1447:1 1449:2 1451:2 1453:2 1455:2 1457:2 1459:1 1460:1 1461:1 1462:1 1463:1 1464:1 1465:1 1466:1 1467:2 1468:1 1469:1 1470:1 1471:1 1472:2 1473:1 1474:1 1475:1 1476:2 1477:1 1478:2 1479:1 1480:1 1481:2 1482:1 1483:2 1484:1 1485:2 1486:1"},
		//large, dense, odd
		{lower: 1e3, upper: 1e5, count: 1e6 - 1, exp: "1784:158 1785:162 1786:164 1787:166 1788:170 1789:171 1790:175 1791:177 1792:180 1793:183 1794:185 1795:189 1796:191 1797:195 1798:197 1799:201 1800:203 1801:207 1802:210 1803:214 1804:217 1805:220 1806:223 1807:227 1808:231 1809:234 1810:238 1811:242 1812:245 1813:249 1814:253 1815:257 1816:261 1817:265 1818:270 1819:273 1820:278 1821:282 1822:287 1823:291 1824:295 1825:300 1826:305 1827:310 1828:314 1829:320 1830:324 1831:329 1832:335 1833:340 1834:345 1835:350 1836:356 1837:362 1838:367 1839:373 1840:379 1841:384 1842:391 1843:397 1844:403 1845:409 1846:416 1847:422 1848:429 1849:435 1850:442 1851:449 1852:457 1853:463 1854:470 1855:478 1856:486 1857:493 1858:500 1859:509 1860:516 1861:525 1862:532 1863:541 1864:550 1865:558 1866:567 1867:575 1868:585 1869:594 1870:603 1871:612 1872:622 1873:632 1874:642 1875:651 1876:662 1877:672 1878:683 1879:693 1880:704 1881:716 1882:726 1883:738 1884:749 1885:761 1886:773 1887:785 1888:797 1889:809 1890:823 1891:835 1892:848 1893:861 1894:875 1895:889 1896:902 1897:917 1898:931 1899:945 1900:960 1901:975 1902:991 1903:1006 1904:1021 1905:1038 1906:1053 1907:1071 1908:1087 1909:1104 1910:1121 1911:1138 1912:1157 1913:1175 1914:1192 1915:1212 1916:1231 1917:1249 1918:1269 1919:1290 1920:1309 1921:1329 1922:1351 1923:1371 1924:1393 1925:1415 1926:1437 1927:1459 1928:1482 1929:1506 1930:1529 1931:1552 1932:1577 1933:1602 1934:1626 1935:1652 1936:1678 1937:1704 1938:1731 1939:1758 1940:1785 1941:1813 1942:1841 1943:1870 1944:1900 1945:1929 1946:1959 1947:1990 1948:2021 1949:2052 1950:2085 1951:2117 1952:2150 1953:2184 1954:2218 1955:2253 1956:2287 1957:2324 1958:2360 1959:2396 1960:2435 1961:2472 1962:2511 1963:2550 1964:2589 1965:2631 1966:2671 1967:2714 1968:2755 1969:2799 1970:2842 1971:2887 1972:2932 1973:2978 1974:3024 1975:3071 1976:3120 1977:3168 1978:3218 1979:3268 1980:3319 1981:3371 1982:3423 1983:3477 1984:3532 1985:3586 1986:3643 1987:3700 1988:3757 1989:3816 1990:3876 1991:3936 1992:3998 1993:4060 1994:4124 1995:4188 1996:4253 1997:4320 1998:4388 1999:4456 2000:4526 2001:4596 2002:4668 2003:4741 2004:4816 2005:4890 2006:4967 2007:5044 2008:5124 2009:5203 2010:5285 2011:5367 2012:5451 2013:5536 2014:5623 2015:5711 2016:5800 2017:5890 2018:5983 2019:6076 2020:6171 2021:6267 2022:6365 2023:6465 2024:6566 2025:6668 2026:6773 2027:6878 2028:6986 2029:7095 2030:7206 2031:7318 2032:7433 2033:7549 2034:7667 2035:7786 2036:7909 2037:8032 2038:8157 2039:8285 2040:8414 2041:8546 2042:8679 2043:8815 2044:8953 2045:9092 2046:9235 2047:9379 2048:9525 2049:9675 2050:9825 2051:9979 2052:10135 2053:10293 2054:10454 2055:10618 2056:10783 2057:10952 2058:11123 2059:11297 2060:11473 2061:11653 2062:11834 2063:12020 2064:12207 2065:12398 2066:12592 2067:12788 2068:12989 2069:13191 2070:13397 2071:13607 2072:13819 2073:14036 2074:14254 2075:14478 2076:14703 2077:14933 2078:15167 2079:15403 2080:8942"},
	} {
		a.Reset()
		a.InsertInterpolate(tt.lower, tt.upper, tt.count)
		check(t, tt)
	}
}
