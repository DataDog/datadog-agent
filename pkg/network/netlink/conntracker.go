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
	"github.com/golang/groupcache/lru"
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
	key connKey
	ttl time.Time
}

type realConntracker struct {
	sync.RWMutex
	consumer      *Consumer
	cache         *lru.Cache
	orphans       *list.List
	orphanTimeout time.Duration

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
		cache:         lru.New(maxStateSize),
		orphans:       list.New(),
		maxStateSize:  maxStateSize,
		compactTicker: time.NewTicker(compactInterval),
		orphanTimeout: defaultOrphanTimeout,
	}

	ctr.cache.OnEvicted = func(key lru.Key, value interface{}) {
		// ctr lock should be held, so it is
		// safe to modify ctr
		t := value.(*translationEntry)
		if t.orphan != nil {
			ctr.orphans.Remove(t.orphan)
		}
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
		now := time.Now().UnixNano()
		atomic.AddInt64(&ctr.stats.gets, 1)
		atomic.AddInt64(&ctr.stats.getTimeTotal, now-then)
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

	v, ok := ctr.cache.Get(k)
	if !ok {
		return nil
	}

	t := v.(*translationEntry)
	if t.orphan != nil {
		ctr.orphans.Remove(t.orphan)
		t.orphan = nil
	}

	return t.IPTranslation
}

func (ctr *realConntracker) GetStats() map[string]int64 {
	// only a few stats are locked
	ctr.RLock()
	size := ctr.cache.Len()
	orphanSize := ctr.orphans.Len()
	ctr.RUnlock()

	m := map[string]int64{
		"state_size":  int64(size),
		"orphan_size": int64(orphanSize),
	}

	if ctr.stats.gets != 0 {
		m["gets_total"] = ctr.stats.gets
		m["nanoseconds_per_get"] = ctr.stats.getTimeTotal / ctr.stats.gets
	}
	if ctr.stats.registers != 0 {
		m["registers_total"] = ctr.stats.registers
		m["registers_dropped"] = ctr.stats.registersDropped
		m["nanoseconds_per_register"] = ctr.stats.registersTotalTime / ctr.stats.registers
	}
	if ctr.stats.unregisters != 0 {
		m["unregisters_total"] = ctr.stats.unregisters
		m["nanoseconds_per_unregister"] = ctr.stats.unregistersTotalTime / ctr.stats.unregisters
	}

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

	_, ok := ctr.cache.Get(k)
	if ok {
		ctr.cache.Remove(k)
		atomic.AddInt64(&ctr.stats.unregisters, 1)
	}
}

func (ctr *realConntracker) Close() {
	ctr.consumer.Stop()
	ctr.compactTicker.Stop()
}

func (ctr *realConntracker) loadInitialState(events <-chan Event) {
	for e := range events {
		conns := DecodeAndReleaseEvent(e)
		for _, c := range conns {
			if !IsNAT(c) {
				continue
			}

			ctr.add(c, false)
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
	defer func() {
		atomic.AddInt64(&ctr.stats.registers, 2)
		atomic.AddInt64(&ctr.stats.registersTotalTime, time.Now().UnixNano()-then)
	}()

	ctr.Lock()
	defer ctr.Unlock()

	ctr.add(c, true)

	return 0
}

func (ctr *realConntracker) add(c Con, orphan bool) {
	registerTuple := func(keyTuple, transTuple *ct.IPTuple) {
		key, ok := formatKey(keyTuple)
		if !ok {
			return
		}

		if v, ok := ctr.cache.Get(key); ok {
			// value is going to get replaced
			// by the call to Add below, make
			// sure orphan is removed
			t := v.(*translationEntry)
			if t.orphan != nil {
				ctr.orphans.Remove(t.orphan)
			}
		}

		t := &translationEntry{
			IPTranslation: formatIPTranslation(transTuple),
		}
		if orphan {
			t.orphan = ctr.orphans.PushFront(&orphanEntry{
				key: key,
				ttl: time.Now().Add(ctr.orphanTimeout),
			})
		}

		ctr.cache.Add(key, t)
	}

	log.Tracef("%s", c)

	registerTuple(c.Origin, c.Reply)
	registerTuple(c.Reply, c.Origin)
}

func (ctr *realConntracker) run() error {
	events, err := ctr.consumer.Events()
	if err != nil {
		return err
	}

	go func() {
		for e := range events {
			conns := DecodeAndReleaseEvent(e)
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

	removed = ctr.removeOrphans(time.Now())
}

func (ctr *realConntracker) removeOrphans(now time.Time) (removed int64) {
	for b := ctr.orphans.Back(); b != nil; b = ctr.orphans.Back() {
		o := b.Value.(*orphanEntry)
		if !o.ttl.Before(now) {
			break
		}

		ctr.cache.Remove(o.key)
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
