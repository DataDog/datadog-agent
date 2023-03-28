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
	nettelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	compactInterval      = time.Minute
	defaultOrphanTimeout = 2 * time.Minute
	telemetryModuleName  = "network_tracer__conntracker"
)

// Conntracker is a wrapper around go-conntracker that keeps a record of all connections in user space
type Conntracker interface {
	GetTranslationForConn(network.ConnectionStats) *network.IPTranslation
	DeleteTranslation(network.ConnectionStats)
	DumpCachedTable(context.Context) (map[uint32][]DebugConntrackEntry, error)
	RefreshTelemetry()
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
	unregistersTotal     *atomic.Int64
	unregistersTotalTime *atomic.Int64
}

type realConntracker struct {
	sync.RWMutex
	consumer *Consumer
	cache    *conntrackCache
	decoder  *Decoder

	// The maximum size the state map will grow before we reject new entries
	maxStateSize int

	compactTicker *time.Ticker
	exit          chan struct{}
	stats         stats
}

var conntrackerTelemetry = struct {
	gets                     telemetry.Histogram
	registers                telemetry.Histogram
	evictsTotal              *nettelemetry.StatCounterWrapper
	registersDropped         *nettelemetry.StatCounterWrapper
	nanoSecondsPerRegister   telemetry.Gauge
	nanoSecondsPerUnRegister telemetry.Gauge
	stateSize                telemetry.Gauge
	orphanSize               telemetry.Gauge
}{
	telemetry.NewHistogram(telemetryModuleName, "gets", []string{}, "Histogram measuring the time spent retrieving connection tuples in the EBPF map", []float64{750, 900, 1000, 1250, 1500, 2500, 5000, 10000}),
	telemetry.NewHistogram(telemetryModuleName, "registers", []string{}, "Histogram measuring the time spent updating/creating connection tuples in the EBPF map", []float64{50, 100, 250, 500, 750, 1000}),
	nettelemetry.NewStatCounterWrapper(telemetryModuleName, "evicts_total", []string{}, "Counter measuring the number of evictions from the conntrack cache"),
	nettelemetry.NewStatCounterWrapper(telemetryModuleName, "registers_dropped", []string{}, "Counter measuring the number of skipped registers due to a non-NAT connection"),
	telemetry.NewGauge(telemetryModuleName, "nanoseconds_per_register", []string{}, "Counter measuring the time spent updating/creating a single connection tuple in the EBPF map"),
	telemetry.NewGauge(telemetryModuleName, "nanoseconds_per_unregister", []string{}, "Gauge measuring the time spent deleting a single connection tuple from the EBPF map"),
	telemetry.NewGauge(telemetryModuleName, "state_size", []string{}, "Counter measuring the current size of the conntrack cache"),
	telemetry.NewGauge(telemetryModuleName, "orphan_size", []string{}, "Counter measuring the number of orphaned items in the conntrack cache"),
}

// NewConntracker creates a new conntracker with a short term buffer capped at the given size
func NewConntracker(config *config.Config) (Conntracker, error) {
	var (
		err         error
		conntracker Conntracker
	)

	done := make(chan struct{})

	go func() {
		conntracker, err = newConntrackerOnce(config)
		done <- struct{}{}
	}()

	select {
	case <-done:
		return conntracker, err
	case <-time.After(config.ConntrackInitTimeout):
		return nil, fmt.Errorf("could not initialize conntrack after: %s", config.ConntrackInitTimeout)
	}
}

func newStats() stats {
	return stats{
		unregistersTotal:     atomic.NewInt64(0),
		unregistersTotalTime: atomic.NewInt64(0),
	}
}

func newConntrackerOnce(cfg *config.Config) (Conntracker, error) {
	consumer, err := NewConsumer(cfg)
	if err != nil {
		return nil, err
	}

	ctr := &realConntracker{
		consumer:      consumer,
		cache:         newConntrackCache(cfg.ConntrackMaxStateSize, defaultOrphanTimeout),
		maxStateSize:  cfg.ConntrackMaxStateSize,
		compactTicker: time.NewTicker(compactInterval),
		exit:          make(chan struct{}),
		decoder:       NewDecoder(),
		stats:         newStats(),
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

	log.Infof("initialized conntrack with target_rate_limit=%d messages/sec", cfg.ConntrackRateLimit)
	return ctr, nil
}

func (ctr *realConntracker) GetTranslationForConn(c network.ConnectionStats) *network.IPTranslation {
	then := time.Now()
	defer func() {
		conntrackerTelemetry.gets.Observe(float64(time.Since(then).Nanoseconds()))
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

// Refreshes conntracker telemetry on a loop
// TODO: Replace with prometheus collector interface
func (ctr *realConntracker) RefreshTelemetry() {
	ticker := time.NewTicker(10 * time.Second)
	for {
		select {
		case <-ticker.C:
			ctr.RLock()
			conntrackerTelemetry.stateSize.Set(float64(ctr.cache.cache.Len()))
			conntrackerTelemetry.orphanSize.Set(float64(ctr.cache.orphans.Len()))
			ctr.RUnlock()
		case <-ctr.exit:
			ticker.Stop()
			return
		}
	}
}

func (ctr *realConntracker) DeleteTranslation(c network.ConnectionStats) {
	then := time.Now().UnixNano()

	ctr.Lock()
	defer ctr.Unlock()

	k := connKey{
		src:       netip.AddrPortFrom(ipFromAddr(c.Source), c.SPort),
		dst:       netip.AddrPortFrom(ipFromAddr(c.Dest), c.DPort),
		transport: c.Type,
	}

	if ctr.cache.Remove(k) {
		ctr.stats.unregistersTotal.Inc()
		ctr.stats.unregistersTotalTime.Add(time.Now().UnixNano() - then)
		if unregisters := ctr.stats.unregistersTotal.Load(); unregisters != 0 {
			conntrackerTelemetry.nanoSecondsPerUnRegister.Set(float64(ctr.stats.unregistersTotalTime.Load() / unregisters))
		}
	}
}

func (ctr *realConntracker) Close() {
	ctr.consumer.Stop()
	ctr.compactTicker.Stop()
	ctr.cache.Purge()
	ctr.exit <- struct{}{}
}

func (ctr *realConntracker) loadInitialState(events <-chan Event) {
	for e := range events {
		conns := ctr.decoder.DecodeAndReleaseEvent(e)
		for _, c := range conns {
			if !IsNAT(c) {
				continue
			}
			then := time.Now()

			evicts := ctr.cache.Add(c, false)

			conntrackerTelemetry.registers.Observe(float64(time.Since(then)))
			conntrackerTelemetry.evictsTotal.Add(int64(evicts))
		}
	}
}

// register is registered to be called whenever a conntrack update/create is called.
// it will keep being called until it returns nonzero.
func (ctr *realConntracker) register(c Con) int {
	// don't bother storing if the connection is not NAT
	if !IsNAT(c) {
		conntrackerTelemetry.registersDropped.Inc()
		return 0
	}
	then := time.Now()

	ctr.Lock()
	defer ctr.Unlock()

	evicts := ctr.cache.Add(c, true)

	conntrackerTelemetry.registers.Observe(float64(time.Since(then)))
	conntrackerTelemetry.evictsTotal.Add(int64(evicts))

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
		ctr.stats.unregistersTotal.Add(removed)
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

func (cc *conntrackCache) Purge() {
	cc.cache.Purge()
	cc.orphans.Init()
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
