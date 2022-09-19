// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package netlink

import (
	"container/list"
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/simplelru"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	compactInterval      = time.Minute
	defaultOrphanTimeout = 2 * time.Minute
)

// Conntracker is a wrapper around go-conntracker that keeps a record of all connections in user space
type Conntracker interface {
	Start() error
	GetTranslationForConn(network.ConnectionStats) *network.IPTranslation
	DeleteTranslation(network.ConnectionStats)
	IsSampling() bool
	GetStats() map[string]int64
	DumpCachedTable(context.Context) (map[uint32][]DebugConntrackEntry, error)
	Close()
}

type connKey struct {
	src netip.AddrPort
	dst netip.AddrPort

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

type stats struct {
	gets                 *atomic.Int64
	getTimeTotal         *atomic.Int64
	registers            *atomic.Int64
	registersDropped     *atomic.Int64
	registersTotalTime   *atomic.Int64
	unregisters          *atomic.Int64
	unregistersTotalTime *atomic.Int64
	evicts               *atomic.Int64
}

type realConntracker struct {
	sync.RWMutex
	consumer *Consumer
	cache    *conntrackCache
	decoder  *Decoder

	// The maximum size the state map will grow before we reject new entries
	maxStateSize int

	compactTicker *time.Ticker
	stats         stats
	timeout       time.Duration
}

func newStats() stats {
	return stats{
		gets:                 atomic.NewInt64(0),
		getTimeTotal:         atomic.NewInt64(0),
		registers:            atomic.NewInt64(0),
		registersDropped:     atomic.NewInt64(0),
		registersTotalTime:   atomic.NewInt64(0),
		unregisters:          atomic.NewInt64(0),
		unregistersTotalTime: atomic.NewInt64(0),
		evicts:               atomic.NewInt64(0),
	}
}

// NewConntracker creates a new conntracker with a short term buffer capped at the given size
func NewConntracker(cfg *config.Config) (Conntracker, error) {
	consumer, err := NewConsumer(cfg)
	if err != nil {
		return nil, err
	}

	ctr := &realConntracker{
		consumer:      consumer,
		cache:         newConntrackCache(cfg.ConntrackMaxStateSize, defaultOrphanTimeout),
		maxStateSize:  cfg.ConntrackMaxStateSize,
		compactTicker: time.NewTicker(compactInterval),
		decoder:       NewDecoder(),
		stats:         newStats(),
		timeout:       cfg.ConntrackInitTimeout,
	}
	log.Infof("initialized conntrack with target_rate_limit=%d messages/sec", cfg.ConntrackRateLimit)
	return ctr, nil
}

func (ctr *realConntracker) Start() error {
	errchan := make(chan error)
	go func() {
		for _, family := range []uint8{unix.AF_INET, unix.AF_INET6} {
			events, err := ctr.consumer.DumpTable(family)
			if err != nil {
				errchan <- fmt.Errorf("error dumping conntrack table for family %d: %w", family, err)
				return
			}
			ctr.loadInitialState(events)
		}
		errchan <- nil
	}()

	select {
	case err := <-errchan:
		if err != nil {
			return err
		}
	case <-time.After(ctr.timeout):
		return fmt.Errorf("could not initialize conntrack after: %s", ctr.timeout)
	}

	return ctr.run()
}

func (ctr *realConntracker) GetTranslationForConn(c network.ConnectionStats) *network.IPTranslation {
	then := time.Now().UnixNano()
	defer func() {
		ctr.stats.gets.Inc()
		ctr.stats.getTimeTotal.Add(time.Now().UnixNano() - then)
	}()

	ctr.Lock()
	defer ctr.Unlock()

	k := connKey{
		src:       netip.AddrPortFrom(ipFromAddr(c.Source), c.SPort),
		dst:       netip.AddrPortFrom(ipFromAddr(c.Dest), c.DPort),
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

	gets := ctr.stats.gets.Load()
	getTimeTotal := ctr.stats.getTimeTotal.Load()
	m["gets_total"] = gets
	if gets != 0 {
		m["nanoseconds_per_get"] = getTimeTotal / gets
	}

	registers := ctr.stats.registers.Load()
	m["registers_total"] = registers
	registersTotalTime := ctr.stats.registersTotalTime.Load()
	if registers != 0 {
		m["nanoseconds_per_register"] = registersTotalTime / registers
	}

	unregisters := ctr.stats.unregisters.Load()
	unregisterTotalTime := ctr.stats.unregistersTotalTime.Load()
	m["unregisters_total"] = unregisters
	if unregisters != 0 {
		m["nanoseconds_per_unregister"] = unregisterTotalTime / unregisters
	}
	m["evicts_total"] = ctr.stats.evicts.Load()

	// Merge telemetry from the consumer
	for k, v := range ctr.consumer.GetStats() {
		m[k] = v
	}

	return m
}

func (ctr *realConntracker) DeleteTranslation(c network.ConnectionStats) {
	then := time.Now().UnixNano()
	defer func() {
		ctr.stats.unregistersTotalTime.Add(time.Now().UnixNano() - then)
	}()

	ctr.Lock()
	defer ctr.Unlock()

	k := connKey{
		src:       netip.AddrPortFrom(ipFromAddr(c.Source), c.SPort),
		dst:       netip.AddrPortFrom(ipFromAddr(c.Dest), c.DPort),
		transport: c.Type,
	}

	if ctr.cache.Remove(k) {
		ctr.stats.unregisters.Inc()
	}
}

func (ctr *realConntracker) IsSampling() bool {
	return ctr.consumer.GetStats()[samplingPct] < 100
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
			ctr.stats.registers.Inc()
			ctr.stats.evicts.Add(int64(evicts))
		}
	}
}

// register is registered to be called whenever a conntrack update/create is called.
// it will keep being called until it returns nonzero.
func (ctr *realConntracker) register(c Con) int {
	// don't bother storing if the connection is not NAT
	if !IsNAT(c) {
		ctr.stats.registersDropped.Inc()
		return 0
	}

	then := time.Now().UnixNano()

	ctr.Lock()
	defer ctr.Unlock()

	evicts := ctr.cache.Add(c, true)

	ctr.stats.registers.Inc()
	ctr.stats.evicts.Add(int64(evicts))
	ctr.stats.registersTotalTime.Add(time.Now().UnixNano() - then)

	return 0
}

func (ctr *realConntracker) run() error {
	events, err := ctr.consumer.Events()
	if err != nil {
		return err
	}

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ctr.compactTicker.C:
				ctr.compact()
			}
		}
	}()

	go func() {
		defer close(done)
		for e := range events {
			conns := ctr.decoder.DecodeAndReleaseEvent(e)
			for _, c := range conns {
				ctr.register(c)
			}
		}
	}()
	return nil
}

func (ctr *realConntracker) compact() {
	var removed int64
	defer func() {
		ctr.stats.unregisters.Add(removed)
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
	registerTuple := func(keyTuple, transTuple *ConTuple) {
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

	registerTuple(&c.Origin, &c.Reply)
	registerTuple(&c.Reply, &c.Origin)
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
	if AddrPortIsZero(c.Origin.Src) ||
		AddrPortIsZero(c.Reply.Src) ||
		c.Origin.Proto == 0 ||
		c.Reply.Proto == 0 ||
		c.Origin.Src.Port() == 0 ||
		c.Origin.Dst.Port() == 0 ||
		c.Reply.Src.Port() == 0 ||
		c.Reply.Dst.Port() == 0 {
		return false
	}

	return c.Origin.Src.Addr() != c.Reply.Dst.Addr() ||
		c.Origin.Dst.Addr() != c.Reply.Src.Addr() ||
		c.Origin.Src.Port() != c.Reply.Dst.Port() ||
		c.Origin.Dst.Port() != c.Reply.Src.Port()
}

func formatIPTranslation(tuple *ConTuple) *network.IPTranslation {
	return &network.IPTranslation{
		ReplSrcIP:   addrFromIP(tuple.Src.Addr()),
		ReplDstIP:   addrFromIP(tuple.Dst.Addr()),
		ReplSrcPort: tuple.Src.Port(),
		ReplDstPort: tuple.Dst.Port(),
	}
}

func addrFromIP(ip netip.Addr) util.Address {
	if ip.Is6() && !ip.Is4In6() {
		b := ip.As16()
		return util.V6AddressFromBytes(b[:])
	}
	b := ip.As4()
	return util.V4AddressFromBytes(b[:])
}

func ipFromAddr(a util.Address) netip.Addr {
	if a.Len() == net.IPv6len {
		return netip.AddrFrom16(*(*[16]byte)(a.Bytes()))
	}
	return netip.AddrFrom4(*(*[4]byte)(a.Bytes()))
}

func formatKey(tuple *ConTuple) (k connKey, ok bool) {
	ok = true
	k.src = tuple.Src
	k.dst = tuple.Dst

	proto := tuple.Proto
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
