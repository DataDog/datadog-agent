// +build linux
// +build !android

package netlink

import (
	"container/list"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ct "github.com/florianl/go-conntrack"
	"github.com/hashicorp/golang-lru/simplelru"
	"golang.org/x/sys/unix"
)

const (
	initializationTimeout = time.Second * 10

	compactInterval      = time.Minute
	defaultOrphanTimeout = 2 * time.Minute
)

// Conntracker is a wrapper around go-conntracker that keeps a record of all connections in user space
type Conntracker interface {
	GetTranslationForConn(network.ConnectionStats) *network.IPTranslation
	DeleteTranslation(network.ConnectionStats)
	GetStats() map[string]int64
	Close()
}

type connKey struct {
	srcIP   util.Address
	srcPort uint16

	dstIP   util.Address
	dstPort uint16

	// the transport protocol of the connection, using the same values as specified in the agent payload.
	transport network.ConnectionType
}

type translationEntry struct {
	*network.IPTranslation
	orphan *list.Element
}

type orphanEntry struct {
	key     connKey
	expires time.Time
}

type realConntracker struct {
	sync.RWMutex
	consumer *Consumer
	cache    *conntrackCache
	decoder  *Decoder

	// The maximum size the state map will grow before we reject new entries
	maxStateSize int

	compactTicker *time.Ticker
	stats         struct {
		gets                 int64
		getTimeTotal         int64
		registers            int64
		registersDropped     int64
		registersTotalTime   int64
		unregisters          int64
		unregistersTotalTime int64
		evicts               int64
	}
}

// NewConntracker creates a new conntracker with a short term buffer capped at the given size
func NewConntracker(config *config.Config) (Conntracker, error) {
	var (
		err         error
		conntracker Conntracker
	)

	done := make(chan struct{})

	go func() {
		conntracker, err = newConntrackerOnce(config.ProcRoot, config.ConntrackMaxStateSize, config.ConntrackRateLimit, config.EnableConntrackAllNamespaces)
		done <- struct{}{}
	}()

	select {
	case <-done:
		return conntracker, err
	case <-time.After(initializationTimeout):
		return nil, fmt.Errorf("could not initialize conntrack after: %s", initializationTimeout)
	}
}

func newConntrackerOnce(procRoot string, maxStateSize, targetRateLimit int, listenAllNamespaces bool) (Conntracker, error) {
	consumer := NewConsumer(procRoot, targetRateLimit, listenAllNamespaces)
	ctr := &realConntracker{
		consumer:      consumer,
		cache:         newConntrackCache(maxStateSize, defaultOrphanTimeout),
		maxStateSize:  maxStateSize,
		compactTicker: time.NewTicker(compactInterval),
		decoder:       NewDecoder(),
	}

	for _, family := range []uint8{unix.AF_INET, unix.AF_INET6} {
		events, err := consumer.DumpTable(family)
		if err != nil {
			return nil, fmt.Errorf("error dumping conntrack table for family %d: %w", family, err)
		}
		ctr.loadInitialState(events)
	}

	if err := ctr.run(); err != nil {
		return nil, err
	}

	log.Infof("initialized conntrack with target_rate_limit=%d messages/sec", targetRateLimit)
	return ctr, nil
}

func (ctr *realConntracker) GetTranslationForConn(c network.ConnectionStats) *network.IPTranslation {
	then := time.Now().UnixNano()
	defer func() {
		atomic.AddInt64(&ctr.stats.gets, 1)
		atomic.AddInt64(&ctr.stats.getTimeTotal, time.Now().UnixNano()-then)
	}()

	ctr.Lock()
	defer ctr.Unlock()

	k := connKey{
		srcIP:     c.Source,
		srcPort:   c.SPort,
		dstIP:     c.Dest,
		dstPort:   c.DPort,
		transport: c.Type,
	}

	t, ok := ctr.cache.Get(k)
	if !ok {
		return nil
	}

	return t.IPTranslation
}

func (ctr *realConntracker) GetStats() map[string]int64 {
	// only a few stats are locked
	ctr.RLock()
	size := ctr.cache.cache.Len()
	orphanSize := ctr.cache.orphans.Len()
	ctr.RUnlock()

	m := map[string]int64{
		"state_size":  int64(size),
		"orphan_size": int64(orphanSize),
	}

	gets := atomic.LoadInt64(&ctr.stats.gets)
	if gets != 0 {
		getTimeTotal := atomic.LoadInt64(&ctr.stats.getTimeTotal)
		m["gets_total"] = gets
		m["nanoseconds_per_get"] = getTimeTotal / gets
	}

	registers := atomic.LoadInt64(&ctr.stats.registers)
	if registers != 0 {
		registersTotalTime := atomic.LoadInt64(&ctr.stats.registersTotalTime)
		m["registers_total"] = registers
		m["nanoseconds_per_register"] = registersTotalTime / registers
	}
	m["registers_dropped"] = atomic.LoadInt64(&ctr.stats.registersDropped)

	unregisters := atomic.LoadInt64(&ctr.stats.unregisters)
	if unregisters != 0 {
		unregisterTotalTime := atomic.LoadInt64(&ctr.stats.unregistersTotalTime)
		m["unregisters_total"] = unregisters
		m["nanoseconds_per_unregister"] = unregisterTotalTime / unregisters
	}
	m["evicts_total"] = atomic.LoadInt64(&ctr.stats.evicts)

	// Merge telemetry from the consumer
	for k, v := range ctr.consumer.GetStats() {
		m[k] = v
	}

	return m
}

func (ctr *realConntracker) DeleteTranslation(c network.ConnectionStats) {
	then := time.Now().UnixNano()
	defer func() {
		atomic.AddInt64(&ctr.stats.unregistersTotalTime, time.Now().UnixNano()-then)
	}()

	ctr.Lock()
	defer ctr.Unlock()

	k := connKey{
		srcIP:     c.Source,
		srcPort:   c.SPort,
		dstIP:     c.Dest,
		dstPort:   c.DPort,
		transport: c.Type,
	}

	if ctr.cache.Remove(k) {
		atomic.AddInt64(&ctr.stats.unregisters, 1)
	}
}

func (ctr *realConntracker) Close() {
	ctr.consumer.Stop()
	ctr.compactTicker.Stop()
}

func (ctr *realConntracker) loadInitialState(events <-chan Event) {
	for e := range events {
		conns := ctr.decoder.DecodeAndReleaseEvent(e)
		for _, c := range conns {
			if !IsNAT(c) {
				continue
			}

			evicts := ctr.cache.Add(c, false)
			atomic.AddInt64(&ctr.stats.registers, 1)
			atomic.AddInt64(&ctr.stats.evicts, int64(evicts))
		}
	}
}

// register is registered to be called whenever a conntrack update/create is called.
// it will keep being called until it returns nonzero.
func (ctr *realConntracker) register(c Con) int {
	// don't bother storing if the connection is not NAT
	if !IsNAT(c) {
		atomic.AddInt64(&ctr.stats.registersDropped, 1)
		return 0
	}

	then := time.Now().UnixNano()

	ctr.Lock()
	defer ctr.Unlock()

	evicts := ctr.cache.Add(c, true)

	atomic.AddInt64(&ctr.stats.registers, 1)
	atomic.AddInt64(&ctr.stats.evicts, int64(evicts))
	atomic.AddInt64(&ctr.stats.registersTotalTime, time.Now().UnixNano()-then)

	return 0
}

func (ctr *realConntracker) run() error {
	events, err := ctr.consumer.Events()
	if err != nil {
		return err
	}

	go func() {
		for e := range events {
			conns := ctr.decoder.DecodeAndReleaseEvent(e)
			for _, c := range conns {
				ctr.register(c)
			}
		}
	}()

	go func() {
		for range ctr.compactTicker.C {
			ctr.compact()
		}
	}()

	return nil
}

func (ctr *realConntracker) compact() {
	var removed int64
	defer func() {
		atomic.AddInt64(&ctr.stats.unregisters, removed)
		log.Debugf("removed %d orphans", removed)
	}()

	ctr.Lock()
	defer ctr.Unlock()

	removed = ctr.cache.removeOrphans(time.Now())
}

type conntrackCache struct {
	cache         *simplelru.LRU
	orphans       *list.List
	orphanTimeout time.Duration
}

func newConntrackCache(maxSize int, orphanTimeout time.Duration) *conntrackCache {
	c := &conntrackCache{
		orphans:       list.New(),
		orphanTimeout: orphanTimeout,
	}

	c.cache, _ = simplelru.NewLRU(maxSize, func(key, value interface{}) {
		t := value.(*translationEntry)
		if t.orphan != nil {
			c.orphans.Remove(t.orphan)
		}
	})

	return c
}

func (cc *conntrackCache) Get(k connKey) (*translationEntry, bool) {
	v, ok := cc.cache.Get(k)
	if !ok {
		return nil, false
	}

	t := v.(*translationEntry)
	if t.orphan != nil {
		cc.orphans.Remove(t.orphan)
		t.orphan = nil
	}

	return t, true
}

func (cc *conntrackCache) Remove(k connKey) bool {
	return cc.cache.Remove(k)
}

func (cc *conntrackCache) Add(c Con, orphan bool) (evicts int) {
	registerTuple := func(keyTuple, transTuple *ct.IPTuple) {
		key, ok := formatKey(keyTuple)
		if !ok {
			return
		}

		if v, ok := cc.cache.Peek(key); ok {
			// value is going to get replaced
			// by the call to Add below, make
			// sure orphan is removed
			t := v.(*translationEntry)
			if t.orphan != nil {
				cc.orphans.Remove(t.orphan)
			}
		}

		t := &translationEntry{
			IPTranslation: formatIPTranslation(transTuple),
		}
		if orphan {
			t.orphan = cc.orphans.PushFront(&orphanEntry{
				key:     key,
				expires: time.Now().Add(cc.orphanTimeout),
			})
		}

		if cc.cache.Add(key, t) {
			evicts++
		}
	}

	log.Tracef("%s", c)

	registerTuple(c.Origin, c.Reply)
	registerTuple(c.Reply, c.Origin)
	return
}

func (cc *conntrackCache) Len() int {
	return cc.cache.Len()
}

func (cc *conntrackCache) removeOrphans(now time.Time) (removed int64) {
	for b := cc.orphans.Back(); b != nil; b = cc.orphans.Back() {
		o := b.Value.(*orphanEntry)
		if !o.expires.Before(now) {
			break
		}

		cc.cache.Remove(o.key)
		removed++
		log.Tracef("removed orphan %+v", o.key)
	}

	return removed
}

// IsNAT returns whether this Con represents a NAT translation
func IsNAT(c Con) bool {
	if c.Origin == nil ||
		c.Reply == nil ||
		c.Origin.Proto == nil ||
		c.Reply.Proto == nil ||
		c.Origin.Proto.SrcPort == nil ||
		c.Origin.Proto.DstPort == nil ||
		c.Reply.Proto.SrcPort == nil ||
		c.Reply.Proto.DstPort == nil {
		return false
	}

	return !(*c.Origin.Src).Equal(*c.Reply.Dst) ||
		!(*c.Origin.Dst).Equal(*c.Reply.Src) ||
		*c.Origin.Proto.SrcPort != *c.Reply.Proto.DstPort ||
		*c.Origin.Proto.DstPort != *c.Reply.Proto.SrcPort
}

func formatIPTranslation(tuple *ct.IPTuple) *network.IPTranslation {
	srcIP := *tuple.Src
	dstIP := *tuple.Dst

	srcPort := *tuple.Proto.SrcPort
	dstPort := *tuple.Proto.DstPort

	return &network.IPTranslation{
		ReplSrcIP:   util.AddressFromNetIP(srcIP),
		ReplDstIP:   util.AddressFromNetIP(dstIP),
		ReplSrcPort: srcPort,
		ReplDstPort: dstPort,
	}
}

func formatKey(tuple *ct.IPTuple) (k connKey, ok bool) {
	ok = true
	k.srcIP = util.AddressFromNetIP(*tuple.Src)
	k.dstIP = util.AddressFromNetIP(*tuple.Dst)
	k.srcPort = *tuple.Proto.SrcPort
	k.dstPort = *tuple.Proto.DstPort

	proto := *tuple.Proto.Number
	switch proto {
	case unix.IPPROTO_TCP:
		k.transport = network.TCP
	case unix.IPPROTO_UDP:
		k.transport = network.UDP
	default:
		ok = false
	}

	return
}
