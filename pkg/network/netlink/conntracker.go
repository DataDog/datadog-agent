// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syndtr/gocapability/capability"
	"golang.org/x/sys/unix"

	"github.com/cihub/seelog"
	"github.com/hashicorp/golang-lru/v2/simplelru"

	telemetryComp "github.com/DataDog/datadog-agent/comp/core/telemetry"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	compactInterval      = time.Minute
	defaultOrphanTimeout = 2 * time.Minute
	telemetryModuleName  = "network_tracer__conntracker"
)

var defaultBuckets = []float64{10, 25, 50, 75, 100, 250, 500, 1000, 10000}

// ErrNotPermitted is the error returned when the current process does not have the required permissions for netlink conntracker
var ErrNotPermitted = errors.New("netlink conntracker requires NET_ADMIN capability")

// Conntracker is a wrapper around go-conntracker that keeps a record of all connections in user space
type Conntracker interface {
	// Describe returns all descriptions of the collector
	Describe(descs chan<- *prometheus.Desc)
	// Collect returns the current state of all metrics of the collector
	Collect(metrics chan<- prometheus.Metric)
	GetTranslationForConn(*network.ConnectionTuple) *network.IPTranslation
	// GetType returns a string describing whether the conntracker is "ebpf" or "netlink"
	GetType() string
	DeleteTranslation(*network.ConnectionTuple)
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

type realConntracker struct {
	sync.RWMutex
	consumer *Consumer
	cache    *conntrackCache
	decoder  *Decoder

	// The maximum size the state map will grow before we reject new entries
	maxStateSize int

	compactTicker *time.Ticker
	exit          chan struct{}
}

var conntrackerTelemetry = struct {
	getsDuration        telemetry.Histogram
	registersDuration   telemetry.Histogram
	unregistersDuration telemetry.Histogram
	getsTotal           telemetry.Counter
	registersTotal      telemetry.Counter
	unregistersTotal    telemetry.Counter
	evictsTotal         telemetry.Counter
	registersDropped    telemetry.Counter
	stateSize           *prometheus.Desc
	orphanSize          *prometheus.Desc
}{
	telemetry.NewHistogram(telemetryModuleName, "gets_duration_nanoseconds", []string{}, "Histogram measuring the time spent retrieving connection tuples in the map", defaultBuckets),
	telemetry.NewHistogram(telemetryModuleName, "registers_duration_nanoseconds", []string{}, "Histogram measuring the time spent updating/creating connection tuples in the map", defaultBuckets),
	telemetry.NewHistogram(telemetryModuleName, "unregisters_duration_nanoseconds", []string{}, "Histogram measuring the time spent removing connection tuples from the map", defaultBuckets),
	telemetry.NewCounter(telemetryModuleName, "gets_total", []string{}, "Counter measuring the total number of attempts to get connection tuples from the map"),
	telemetry.NewCounter(telemetryModuleName, "registers_total", []string{}, "Counter measuring the total number of attempts to update/create connection tuples in the map"),
	telemetry.NewCounter(telemetryModuleName, "unregisters_total", []string{}, "Counter measuring the total number of attempts to delete connection tuples from the map"),
	telemetry.NewCounter(telemetryModuleName, "evicts_total", []string{}, "Counter measuring the number of evictions from the conntrack cache"),
	telemetry.NewCounter(telemetryModuleName, "registers_dropped", []string{}, "Counter measuring the number of skipped registers due to a non-NAT connection"),
	prometheus.NewDesc(telemetryModuleName+"__state_size", "Gauge measuring the current size of the conntrack cache", nil, nil),
	prometheus.NewDesc(telemetryModuleName+"__orphan_size", "Gauge measuring the number of orphaned items in the conntrack cache", nil, nil),
}

// isNetlinkConntrackSupported checks if we have the right capabilities
// for netlink conntrack; NET_ADMIN is required
func isNetlinkConntrackSupported() bool {
	// check if we have the right capabilities for the netlink NewConntracker
	// NET_ADMIN is required
	caps, err := capability.NewPid2(0)
	if err == nil {
		err = caps.Load()
	}

	if err != nil {
		log.Warnf("could not check if netlink conntracker is supported: %s", err)
		return false
	}

	return caps.Get(capability.EFFECTIVE, capability.CAP_NET_ADMIN)
}

// NewConntracker creates a new conntracker with a short term buffer capped at the given size
func NewConntracker(config *config.Config, telemetrycomp telemetryComp.Component) (Conntracker, error) {
	var (
		err         error
		conntracker Conntracker
	)

	if !isNetlinkConntrackSupported() {
		return nil, ErrNotPermitted
	}

	done := make(chan struct{})

	go func() {
		conntracker, err = newConntrackerOnce(config, telemetrycomp)
		done <- struct{}{}
	}()

	select {
	case <-done:
		return conntracker, err
	case <-time.After(config.ConntrackInitTimeout):
		return nil, fmt.Errorf("could not initialize conntrack after: %s", config.ConntrackInitTimeout)
	}
}

func newConntrackerOnce(cfg *config.Config, telemetrycomp telemetryComp.Component) (Conntracker, error) {
	consumer, err := NewConsumer(cfg, telemetrycomp)
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

// GetType returns a string describing whether the conntracker is "ebpf" or "netlink"
func (ctr *realConntracker) GetType() string {
	return "netlink"
}

func (ctr *realConntracker) GetTranslationForConn(c *network.ConnectionTuple) *network.IPTranslation {
	then := time.Now()
	defer func() {
		conntrackerTelemetry.getsDuration.Observe(float64(time.Since(then).Nanoseconds()))
		conntrackerTelemetry.getsTotal.Inc()
	}()

	ctr.Lock()
	defer ctr.Unlock()

	k := connKey{
		src:       netip.AddrPortFrom(c.Source.Addr, c.SPort),
		dst:       netip.AddrPortFrom(c.Dest.Addr, c.DPort),
		transport: c.Type,
	}

	t, ok := ctr.cache.Get(k)
	if !ok {
		return nil
	}

	return t.IPTranslation
}

// Describe returns all descriptions of the collector
func (ctr *realConntracker) Describe(ch chan<- *prometheus.Desc) {
	ch <- conntrackerTelemetry.stateSize
	ch <- conntrackerTelemetry.orphanSize
}

// Collect returns the current state of all metrics of the collector
func (ctr *realConntracker) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(conntrackerTelemetry.stateSize, prometheus.CounterValue, float64(ctr.cache.cache.Len()))
	ch <- prometheus.MustNewConstMetric(conntrackerTelemetry.orphanSize, prometheus.CounterValue, float64(ctr.cache.orphans.Len()))
}

func (ctr *realConntracker) DeleteTranslation(c *network.ConnectionTuple) {
	then := time.Now()

	ctr.Lock()
	defer ctr.Unlock()

	k := connKey{
		src:       netip.AddrPortFrom(c.Source.Addr, c.SPort),
		dst:       netip.AddrPortFrom(c.Dest.Addr, c.DPort),
		transport: c.Type,
	}

	if ctr.cache.Remove(k) {
		conntrackerTelemetry.unregistersDuration.Observe(float64(time.Since(then).Nanoseconds()))
		conntrackerTelemetry.unregistersTotal.Inc()
	}
}

func (ctr *realConntracker) Close() {
	ctr.consumer.Stop()
	ctr.compactTicker.Stop()
	ctr.cache.Purge()
	close(ctr.exit)
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

			conntrackerTelemetry.registersTotal.Inc()
			conntrackerTelemetry.registersDuration.Observe(float64(time.Since(then).Nanoseconds()))
			conntrackerTelemetry.evictsTotal.Add(float64(evicts))
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

	conntrackerTelemetry.registersTotal.Inc()
	conntrackerTelemetry.registersDuration.Observe(float64(time.Since(then).Nanoseconds()))
	conntrackerTelemetry.evictsTotal.Add(float64(evicts))

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
		conntrackerTelemetry.unregistersTotal.Add(float64(removed))
		log.Debugf("removed %d orphans", removed)
	}()

	ctr.Lock()
	defer ctr.Unlock()

	removed = ctr.cache.removeOrphans(time.Now())
}

type conntrackCache struct {
	cache         *simplelru.LRU[connKey, *translationEntry]
	orphans       *list.List
	orphanTimeout time.Duration
}

func newConntrackCache(maxSize int, orphanTimeout time.Duration) *conntrackCache {
	c := &conntrackCache{
		orphans:       list.New(),
		orphanTimeout: orphanTimeout,
	}

	c.cache, _ = simplelru.NewLRU(maxSize, func(_ connKey, t *translationEntry) {
		if t.orphan != nil {
			c.orphans.Remove(t.orphan)
		}
	})

	return c
}

func (cc *conntrackCache) Get(k connKey) (*translationEntry, bool) {
	t, ok := cc.cache.Get(k)
	if !ok {
		return nil, false
	}

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

		if t, ok := cc.cache.Peek(key); ok {
			// value is going to get replaced
			// by the call to Add below, make
			// sure orphan is removed
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

	if log.ShouldLog(seelog.TraceLvl) {
		log.Tracef("%s", c)
	}

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
		if log.ShouldLog(seelog.TraceLvl) {
			log.Tracef("removed orphan %+v", o.key)
		}
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
		ReplSrcIP:   util.Address{Addr: tuple.Src.Addr().Unmap()},
		ReplDstIP:   util.Address{Addr: tuple.Dst.Addr().Unmap()},
		ReplSrcPort: tuple.Src.Port(),
		ReplDstPort: tuple.Dst.Port(),
	}
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
