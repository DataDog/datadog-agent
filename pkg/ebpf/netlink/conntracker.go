// +build linux

package netlink

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	ct "github.com/florianl/go-conntrack"
	"github.com/pkg/errors"
)

const (
	initializationTimeout = time.Second * 10

	compactInterval = time.Minute * 5
)

// Conntracker is a wrapper around go-conntracker that keeps a record of all connections in user space
type Conntracker interface {
	GetTranslationForConn(ip util.Address, port uint16) *IPTranslation
	ClearShortLived()
	GetStats() map[string]int64
	Close()
}

type connKey struct {
	ip   util.Address
	port uint16
}

type connValue struct {
	*IPTranslation
	expGeneration uint8
}

type realConntracker struct {
	sync.Mutex

	// we need two nfct handles because we can only register one callback per connection at a time
	nfct    *ct.Nfct
	nfctDel *ct.Nfct

	state map[connKey]*connValue

	// a short term buffer of connections to IPTranslations. Since we cannot make sure that tracer.go
	// will try to read the translation for an IP before the delete callback happens, we will
	// safe a fixed number of connections
	shortLivedBuffer map[connKey]*IPTranslation

	// the maximum size of the short lived buffer
	maxShortLivedBuffer int

	// The maximum size the state map will grow before we reject new entries
	maxStateSize int

	statsTicker   *time.Ticker
	compactTicker *time.Ticker
	stats         struct {
		gets                 int64
		getTimeTotal         int64
		registers            int64
		registersTotalTime   int64
		unregisters          int64
		unregistersTotalTime int64
		expiresTotal         int64
	}
}

// NewConntracker creates a new conntracker with a short term buffer capped at the given size
func NewConntracker(procRoot string, deleteBufferSize, maxStateSize int) (Conntracker, error) {
	var (
		err         error
		conntracker Conntracker
	)

	done := make(chan struct{})

	go func() {
		conntracker, err = newConntrackerOnce(procRoot, deleteBufferSize, maxStateSize)
		done <- struct{}{}
	}()

	select {
	case <-done:
		return conntracker, err
	case <-time.After(initializationTimeout):
		return nil, fmt.Errorf("could not initialize conntrack after: %s", initializationTimeout)
	}
}

func newConntrackerOnce(procRoot string, deleteBufferSize, maxStateSize int) (Conntracker, error) {
	if deleteBufferSize <= 0 {
		return nil, fmt.Errorf("short term buffer size is less than 0")
	}

	netns := getGlobalNetNSFD(procRoot)

	logger := getLogger()
	nfct, err := ct.Open(&ct.Config{ReadTimeout: 10 * time.Millisecond, NetNS: netns, Logger: logger})
	if err != nil {
		return nil, err
	}

	nfctDel, err := ct.Open(&ct.Config{ReadTimeout: 10 * time.Millisecond, NetNS: netns, Logger: logger})
	if err != nil {
		return nil, errors.Wrap(err, "failed to open delete NFCT")
	}

	ctr := &realConntracker{
		nfct:                nfct,
		nfctDel:             nfctDel,
		compactTicker:       time.NewTicker(compactInterval),
		state:               make(map[connKey]*connValue),
		shortLivedBuffer:    make(map[connKey]*IPTranslation),
		maxShortLivedBuffer: deleteBufferSize,
		maxStateSize:        maxStateSize,
	}

	// seed the state
	sessions, err := nfct.Dump(ct.Ct, ct.CtIPv4)
	if err != nil {
		return nil, err
	}
	ctr.loadInitialState(sessions)
	log.Debugf("seeded IPv4 state")

	sessions, err = nfct.Dump(ct.Ct, ct.CtIPv6)
	if err != nil {
		// this is not fatal because we've already seeded with IPv4
		log.Errorf("Failed to dump IPv6")
	}
	ctr.loadInitialState(sessions)
	log.Debugf("seeded IPv6 state")

	go ctr.run()

	nfct.Register(context.Background(), ct.Ct, ct.NetlinkCtNew|ct.NetlinkCtExpectedNew|ct.NetlinkCtUpdate, ctr.register)
	log.Debugf("initialized register hook")

	nfctDel.Register(context.Background(), ct.Ct, ct.NetlinkCtDestroy, ctr.unregister)
	log.Debugf("initialized unregister hook")

	log.Infof("initialized conntrack")

	return ctr, nil
}

func (ctr *realConntracker) GetTranslationForConn(ip util.Address, port uint16) *IPTranslation {
	then := time.Now().UnixNano()

	ctr.Lock()
	defer ctr.Unlock()

	k := connKey{ip, port}
	var result *IPTranslation
	value, ok := ctr.state[k]
	if !ok {
		result = ctr.shortLivedBuffer[k]
	} else {
		value.expGeneration = getNthGeneration(compactInterval, then, 3)
		result = value.IPTranslation
	}

	now := time.Now().UnixNano()
	atomic.AddInt64(&ctr.stats.gets, 1)
	atomic.AddInt64(&ctr.stats.getTimeTotal, now-then)
	return result
}

func (ctr *realConntracker) ClearShortLived() {
	ctr.Lock()
	defer ctr.Unlock()

	ctr.shortLivedBuffer = make(map[connKey]*IPTranslation, len(ctr.shortLivedBuffer))
}

func (ctr *realConntracker) GetStats() map[string]int64 {
	// only a few stats are locked
	ctr.Lock()
	size := len(ctr.state)
	stBufSize := len(ctr.shortLivedBuffer)
	ctr.Unlock()

	m := map[string]int64{
		"state_size":             int64(size),
		"short_term_buffer_size": int64(stBufSize),
		"expires":                int64(ctr.stats.expiresTotal),
	}

	if ctr.stats.gets != 0 {
		m["gets_total"] = ctr.stats.gets
		m["nanoseconds_per_get"] = ctr.stats.getTimeTotal / ctr.stats.gets
	}
	if ctr.stats.registers != 0 {
		m["registers_total"] = ctr.stats.registers
		m["nanoseconds_per_register"] = ctr.stats.registersTotalTime / ctr.stats.registers
	}
	if ctr.stats.unregisters != 0 {
		m["unregisters_total"] = ctr.stats.unregisters
		m["nanoseconds_per_unregister"] = ctr.stats.unregistersTotalTime / ctr.stats.unregisters

	}

	return m
}

func (ctr *realConntracker) Close() {
	ctr.compactTicker.Stop()
}

func (ctr *realConntracker) loadInitialState(sessions []ct.Conn) {
	gen := getNthGeneration(compactInterval, time.Now().UnixNano(), 3)
	for _, c := range sessions {
		if isNAT(c) {
			ctr.state[formatKey(c)] = formatIPTranslation(c, gen)
		}
	}
}

// register is registered to be called whenever a conntrack update/create is called.
// it will keep being called until it returns nonzero.
func (ctr *realConntracker) register(c ct.Conn) int {
	// don't both storing if the connection is not NAT
	if !isNAT(c) {
		return 0
	}

	now := time.Now().UnixNano()
	ctr.Lock()
	defer ctr.Unlock()

	if len(ctr.state) >= ctr.maxStateSize {
		log.Warnf("exceeded maximum conntrack state size: %d entries", ctr.maxStateSize)
		return 0
	}

	generation := getNthGeneration(compactInterval, now, 3)
	ctr.state[formatKey(c)] = formatIPTranslation(c, generation)

	then := time.Now().UnixNano()
	atomic.AddInt64(&ctr.stats.registers, 1)
	atomic.AddInt64(&ctr.stats.registersTotalTime, then-now)

	return 0
}

// unregister is registered to be called whenever a conntrack entry is destroyed.
// it will keep being called until it returns nonzero.
func (ctr *realConntracker) unregister(c ct.Conn) int {
	if !isNAT(c) {
		return 0
	}

	now := time.Now().UnixNano()

	ctr.Lock()
	defer ctr.Unlock()

	// move the mapping from the permanent to "short lived" connection
	k := formatKey(c)
	translation, ok := ctr.state[k]

	delete(ctr.state, k)
	if len(ctr.shortLivedBuffer) < ctr.maxShortLivedBuffer && ok {
		ctr.shortLivedBuffer[k] = translation.IPTranslation
	} else {
		log.Warn("exceeded maximum tracked short lived connections")
	}

	then := time.Now().UnixNano()
	atomic.AddInt64(&ctr.stats.unregisters, 1)
	atomic.AddInt64(&ctr.stats.unregistersTotalTime, then-now)

	return 0
}

func (ctr *realConntracker) run() {
	for range ctr.compactTicker.C {
		ctr.compact()
	}
}

func (ctr *realConntracker) compact() {
	ctr.Lock()
	defer ctr.Unlock()

	gen := getCurrentGeneration(compactInterval, time.Now().UnixNano())

	// https://github.com/golang/go/issues/20135
	copied := make(map[connKey]*connValue, len(ctr.state))
	for k, v := range ctr.state {
		if v.expGeneration != gen {
			copied[k] = v
		} else {
			atomic.AddInt64(&ctr.stats.expiresTotal, 1)
		}
	}
	ctr.state = copied
}

func isNAT(c ct.Conn) bool {
	originSrcIPv4 := c[ct.AttrOrigIPv4Src]
	originDstIPv4 := c[ct.AttrOrigIPv4Dst]
	replSrcIPv4 := c[ct.AttrReplIPv4Src]
	replDstIPv4 := c[ct.AttrReplIPv4Dst]

	originSrcIPv6 := c[ct.AttrOrigIPv6Src]
	originDstIPv6 := c[ct.AttrOrigIPv6Dst]
	replSrcIPv6 := c[ct.AttrReplIPv6Src]
	replDstIPv6 := c[ct.AttrReplIPv6Dst]

	originSrcPort, _ := c.Uint16(ct.AttrOrigPortSrc)
	originDstPort, _ := c.Uint16(ct.AttrOrigPortDst)
	replSrcPort, _ := c.Uint16(ct.AttrReplPortSrc)
	replDstPort, _ := c.Uint16(ct.AttrReplPortDst)

	return !bytes.Equal(originSrcIPv4, replDstIPv4) ||
		!bytes.Equal(originSrcIPv6, replDstIPv6) ||
		!bytes.Equal(originDstIPv4, replSrcIPv4) ||
		!bytes.Equal(originDstIPv6, replSrcIPv6) ||
		originSrcPort != replDstPort ||
		originDstPort != replSrcPort
}

// ReplSrcIP extracts the source IP of the reply tuple from a conntrack entry
func ReplSrcIP(c ct.Conn) net.IP {
	if ipv4, ok := c[ct.AttrReplIPv4Src]; ok {
		return net.IPv4(ipv4[0], ipv4[1], ipv4[2], ipv4[3])
	}

	if ipv6, ok := c[ct.AttrReplIPv6Src]; ok {
		return net.IP(ipv6)
	}

	return nil
}

// ReplDstIP extracts the dest IP of the reply tuple from a conntrack entry
func ReplDstIP(c ct.Conn) net.IP {
	if ipv4, ok := c[ct.AttrReplIPv4Dst]; ok {
		return net.IPv4(ipv4[0], ipv4[1], ipv4[2], ipv4[3])
	}

	if ipv6, ok := c[ct.AttrReplIPv6Dst]; ok {
		return net.IP(ipv6)
	}

	return nil
}

func formatIPTranslation(c ct.Conn, generation uint8) *connValue {
	replSrcIP := ReplSrcIP(c)
	replDstIP := ReplDstIP(c)

	replSrcPort, err := c.Uint16(ct.AttrReplPortSrc)
	if err != nil {
		return nil
	}

	replDstPort, err := c.Uint16(ct.AttrReplPortDst)
	if err != nil {
		return nil
	}

	return &connValue{
		IPTranslation: &IPTranslation{
			ReplSrcIP:   util.AddressFromNetIP(replSrcIP),
			ReplDstIP:   util.AddressFromNetIP(replDstIP),
			ReplSrcPort: NtohsU16(replSrcPort),
			ReplDstPort: NtohsU16(replDstPort),
		},
		expGeneration: generation,
	}
}

func formatKey(c ct.Conn) (k connKey) {
	if ip, err := c.OrigSrcIP(); err == nil {
		k.ip = util.AddressFromNetIP(ip)
	}
	if port, err := c.Uint16(ct.AttrOrigPortSrc); err == nil {
		k.port = NtohsU16(port)
	}
	return
}
