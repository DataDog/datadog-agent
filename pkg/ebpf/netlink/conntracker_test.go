// +build linux

package netlink

import (
	"encoding/binary"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"

	ct "github.com/florianl/go-conntrack"
	"github.com/stretchr/testify/assert"
)

func TestConntrackExpvar(t *testing.T) {
	rt := newConntracker()
	go rt.run()
	<-time.After(time.Second)
	assert.Equal(t, conntrackExpvar.String(), "{\"short_term_buffer_size\": 0, \"state_size\": 0}")
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
	translation := rt.GetTranslationForConn(util.AddressFromString("10.0.0.0"), 8080)
	assert.Nil(t, translation)
}

func TestRegisterNat(t *testing.T) {
	rt := newConntracker()
	c := makeTranslatedConn("10.0.0.0:12345", "50.30.40.10:80", "20.0.0.0:80")

	rt.register(c)
	translation := rt.GetTranslationForConn(util.AddressFromString("10.0.0.0"), 12345)
	assert.NotNil(t, translation)
	assert.Equal(t, &IPTranslation{
		ReplSrcIP:   "20.0.0.0",
		ReplDstIP:   "10.0.0.0",
		ReplSrcPort: 80,
		ReplDstPort: 12345,
	}, translation)

	// even after unregistering, we should be able to access the conn
	rt.unregister(c)
	translation2 := rt.GetTranslationForConn(util.AddressFromString("10.0.0.0"), 12345)
	assert.NotNil(t, translation2)

	// double unregisters should never happen, though it will be treated as a no-op
	rt.unregister(c)

	rt.ClearShortLived()
	translation3 := rt.GetTranslationForConn(util.AddressFromString("10.0.0.0"), 12345)
	assert.Nil(t, translation3)

	// triple unregisters should never happen, though it will be treated as a no-op
	rt.unregister(c)

	assert.Equal(t, translation, translation2)

}

func newConntracker() *realConntracker {
	return &realConntracker{
		state:               make(map[connKey]*IPTranslation),
		shortLivedBuffer:    make(map[connKey]*IPTranslation),
		maxShortLivedBuffer: 10000,
		compactTicker:       time.NewTicker(time.Hour),
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
