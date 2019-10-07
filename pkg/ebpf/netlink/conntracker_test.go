// +build linux

package netlink

import (
	"encoding/binary"
	"io"
	"io/ioutil"
	"net"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	ct "github.com/florianl/go-conntrack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithIPTables(t *testing.T) {
	assertRoot(t)

	// run the teardown, just in case the last run did not exit cleanly
	runScript(t, "teardown_dnat.sh")

	// setup a dummy interface at 1.1.1.1 and a rule to DNAT 2.2.2.2 ->1.1.1.1
	runScript(t, "setup_dnat.sh")
	defer runScript(t, "teardown_dnat.sh")

	ct, err := NewConntracker("/proc", 10, 1000)
	require.NoError(t, err)
	defer ct.Close()

	// setup a listener on 1.1.1.1 (the real address of the dummy addr)
	tcpListener, err := net.Listen("tcp", "1.1.1.1:8080")
	require.NoError(t, err)
	go func() {
		tcpListener.Accept()
	}()
	defer tcpListener.Close()

	udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.ParseIP("1.1.1.1"),
		Port: 8080,
	})
	defer udpListener.Close()

	_, err = net.Dial("tcp", "2.2.2.2:8080")
	require.NoError(t, err)

	udpConn, err := net.Dial("udp", "2.2.2.2:8080")
	require.NoError(t, err)
	go func() {
		udpConn.Write([]byte("hello"))
	}()

	timeout := time.After(time.Second * 5)
	for {
		select {
		case <-timeout:
			t.Errorf("missing entry")
			t.Fail()
			return
		default:
			var udpFound, tcpFound bool
			for k, v := range ct.(*realConntracker).state {
				if v.ReplSrcIP == util.AddressFromString("1.1.1.1") {
					if k.proto == 0 { // ConnectionType_tcp
						tcpFound = true
					} else if k.proto == 1 { // ConnectionType_udp
						udpFound = true
					} else {
						t.Errorf("unexpected proto: %v", k.proto)
					}
				}
				if udpFound && tcpFound {
					return
				}
			}
		}
	}
}

func TestIsNat(t *testing.T) {
	c := map[ct.ConnAttrType][]byte{
		ct.AttrOrigIPv4Src: {1, 1, 1, 1},
		ct.AttrOrigIPv4Dst: {2, 2, 2, 2},

		ct.AttrReplIPv4Src: {2, 2, 2, 2},
		ct.AttrReplIPv4Dst: {1, 1, 1, 1},
	}
	assert.False(t, isNAT(c))
}

func TestRegisterNonNat(t *testing.T) {
	rt := newConntracker()
	c := makeUntranslatedConn("10.0.0.0:8080", "50.30.40.10:12345")

	rt.register(c)
	translation := rt.GetTranslationForConn(util.AddressFromString("10.0.0.0"), 8080, 0)
	assert.Nil(t, translation)
}

func TestRegisterNat(t *testing.T) {
	rt := newConntracker()
	c := makeTranslatedConn("10.0.0.0:12345", "50.30.40.10:80", "20.0.0.0:80")

	rt.register(c)
	translation := rt.GetTranslationForConn(util.AddressFromString("10.0.0.0"), 12345, 0)
	assert.NotNil(t, translation)
	assert.Equal(t, &IPTranslation{
		ReplSrcIP:   util.AddressFromString("20.0.0.0"),
		ReplDstIP:   util.AddressFromString("10.0.0.0"),
		ReplSrcPort: 80,
		ReplDstPort: 12345,
	}, translation)

	// even after unregistering, we should be able to access the conn
	rt.unregister(c)
	translation2 := rt.GetTranslationForConn(util.AddressFromString("10.0.0.0"), 12345, 0)
	assert.NotNil(t, translation2)

	// double unregisters should never happen, though it will be treated as a no-op
	rt.unregister(c)

	rt.ClearShortLived()
	translation3 := rt.GetTranslationForConn(util.AddressFromString("10.0.0.0"), 12345, 0)
	assert.Nil(t, translation3)

	// triple unregisters should never happen, though it will be treated as a no-op
	rt.unregister(c)

	assert.Equal(t, translation, translation2)

}

func TestGetUpdatesGen(t *testing.T) {
	rt := newConntracker()
	c := makeTranslatedConn("10.0.0.0:12345", "50.30.40.10:80", "20.0.0.0:80")

	rt.register(c)
	var last uint8
	for _, v := range rt.state {
		v.expGeneration -= 5
		last = v.expGeneration
		break // there is only one item in the map
	}

	iptr := rt.GetTranslationForConn(util.AddressFromString("10.0.0.0"), 12345, 0)
	require.NotNil(t, iptr)

	for _, v := range rt.state {
		assert.NotEqual(t, last, v.expGeneration)
		break // there is only one item in the map
	}
}

func TestTooManyEntries(t *testing.T) {
	rt := newConntracker()
	rt.maxStateSize = 1

	rt.register(makeTranslatedConn("10.0.0.0:12345", "50.30.40.10:80", "20.0.0.0:80"))
	rt.register(makeTranslatedConn("10.0.0.1:12345", "50.30.40.10:80", "20.0.0.0:80"))
	rt.register(makeTranslatedConn("10.0.0.2:12345", "50.30.40.10:80", "20.0.0.0:80"))
}

func newConntracker() *realConntracker {
	return &realConntracker{
		state:                make(map[connKey]*connValue),
		shortLivedBuffer:     make(map[connKey]*IPTranslation),
		maxShortLivedBuffer:  10000,
		compactTicker:        time.NewTicker(time.Hour),
		maxStateSize:         10000,
		exceededSizeLogLimit: util.NewLogLimit(1, time.Minute),
	}
}

func makeUntranslatedConn(from, to string) ct.Conn {
	return makeTranslatedConn(from, to, to)
}

// makes a translation where from -> to is shows as actualTo -> from
func makeTranslatedConn(from, to, actualTo string) ct.Conn {
	ip, port := parts(from)
	dip, dport := parts(to)
	tip, tport := parts(actualTo)

	return map[ct.ConnAttrType][]byte{
		ct.AttrOrigIPv4Src: ip,
		ct.AttrOrigPortSrc: port,
		ct.AttrOrigIPv4Dst: dip,
		ct.AttrOrigPortDst: dport,

		ct.AttrReplIPv4Src: tip,
		ct.AttrReplPortSrc: tport,
		ct.AttrReplIPv4Dst: ip,
		ct.AttrReplPortDst: port,
	}
}

// splits an IP:port string into network order byte representations of IP and port.
// IPv4 only.
func parts(p string) ([]byte, []byte) {
	segments := strings.Split(p, ":")
	prt, _ := strconv.ParseUint(segments[1], 10, 16)
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(prt))

	ip := net.ParseIP(segments[0]).To4()

	return ip, b
}

func assertRoot(t *testing.T) {
	user, err := user.Current()
	if err != nil {
		t.Skipf("cannot detect user, will skip test %v", err)
	}

	if user.Username != "root" {
		t.Skipf("skipping test because username is not root, but %v", user.Username)
	}
}

func runScript(t *testing.T, fname string) {
	path := filepath.Join("testdata", fname)
	cmd := exec.Command("/bin/bash", path)
	stdErr, err := cmd.StderrPipe()
	assert.NoError(t, err)

	stdOut, err := cmd.StdoutPipe()
	assert.NoError(t, err)

	cmdOut := io.MultiReader(stdErr, stdOut)
	err = cmd.Start()
	assert.NoError(t, err)

	message, err := ioutil.ReadAll(cmdOut)
	assert.NoError(t, err)

	if err := cmd.Wait(); err != nil {
		t.Errorf("failed with stderr: %v", string(message))
		t.Errorf("failed with error %v", err)
	}
}
