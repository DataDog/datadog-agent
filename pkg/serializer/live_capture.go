// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package serializer

import (
	"sort"
	"strings"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// liveSeriesCapture wraps the normal iterator; it never traverses it separately.
// Every instance belongs to one flush. Route callbacks run synchronously at the
// forwarder's initial enqueue boundary, before finish transfers ownership.
type liveSeriesCapture struct {
	source          metrics.SerieSource
	current         *metrics.Serie
	manager         *telemetrycapture.Manager
	reservation     *telemetrycapture.Reservation
	ordinal         uint64
	selectedOrdinal uint64
	exhausted       bool
	series          []telemetrycapture.Series
	routes          []telemetrycapture.Route
	payloads        []*transaction.BytesPayload
	routed          []bool
}

func beginLiveSeries(m *telemetrycapture.Manager, cadence time.Duration, source metrics.SerieSource) *liveSeriesCapture {
	if !m.Enabled() {
		return nil
	}
	reservation := m.Begin(telemetrycapture.Metrics, time.Now(), cadence, 1024)
	if reservation == nil {
		return nil
	}
	return &liveSeriesCapture{source: source, manager: m, reservation: reservation}
}

func (c *liveSeriesCapture) MoveNext() bool {
	c.selectedOrdinal = 0
	if !c.source.MoveNext() {
		c.exhausted = true
		c.current = nil
		return false
	}
	c.ordinal++
	c.current = c.source.Current()
	v := c.current
	if v == nil || !telemetrycapture.MetricAllowed(v.Name) {
		return true
	}
	c.copyCurrent(v)
	return true
}

func (c *liveSeriesCapture) copyCurrent(v *metrics.Serie) {
	defer func() {
		if recover() != nil {
			_ = c.manager.Fail(c.reservation.Control())
		}
	}()
	// Charge both the eventual owned copy and temporary slice growth before any
	// cloning. Strings must be cloned too: a substring may retain a larger buffer.
	bytes := 4*int64(unsafe.Sizeof(telemetrycapture.Series{})) + int64(len(v.Name)+len(v.Host)+len(v.Device))
	bytes += int64(v.Tags.Len())*int64(unsafe.Sizeof("")) + int64(len(v.Points))*int64(unsafe.Sizeof(telemetrycapture.Point{}))
	v.Tags.ForEach(func(tag string) { bytes += int64(len(tag)) })
	if !c.ReserveCaptureBytes(bytes) {
		return
	}
	copy := telemetrycapture.Series{
		Ordinal: c.ordinal, Name: strings.Clone(v.Name), Source: uint32(v.Source),
		Type: int32(v.MType), Interval: v.Interval, Host: strings.Clone(v.Host), Device: strings.Clone(v.Device),
		Tags: make([]string, 0, v.Tags.Len()), Points: make([]telemetrycapture.Point, len(v.Points)),
	}
	v.Tags.ForEach(func(tag string) { copy.Tags = append(copy.Tags, strings.Clone(tag)) })
	for i, p := range v.Points {
		copy.Points[i] = telemetrycapture.Point{Timestamp: p.Ts, Value: p.Value}
	}
	c.series = append(c.series, copy)
	c.selectedOrdinal = c.ordinal
}

func (c *liveSeriesCapture) Current() *metrics.Serie              { return c.current }
func (c *liveSeriesCapture) Count() uint64                        { return c.source.Count() }
func (c *liveSeriesCapture) CurrentCaptureOrdinal() uint64        { return c.selectedOrdinal }
func (c *liveSeriesCapture) ReserveCaptureBytes(bytes int64) bool { return c.reservation.Grow(bytes) }

func (c *liveSeriesCapture) CapturePayload(payload *transaction.BytesPayload, ordinals []uint64) {
	if !c.ReserveCaptureBytes(512) {
		return
	}
	c.payloads = append(c.payloads, payload)
	c.routed = append(c.routed, false)
	payload.SetCapture(&transaction.CaptureMetadata{
		SessionID: c.reservation.Control().SessionID, CycleID: c.reservation.CycleID(),
		PayloadID: uint64(len(c.payloads)), Ordinals: ordinals, Observer: c,
	})
}

// ObserveRoute receives fixed routing labels, never raw destinations or headers.
func (c *liveSeriesCapture) ObserveRoute(payloadID uint64, ordinals []uint64, endpoint, protocol, destination string, enqueuedAt time.Time) {
	defer func() {
		if recover() != nil {
			_ = c.manager.Fail(c.reservation.Control())
		}
	}()
	if payloadID == 0 || payloadID > uint64(len(c.payloads)) || len(ordinals) == 0 ||
		!strings.HasPrefix(endpoint, "/") || strings.ContainsAny(endpoint, "?#") || protocol == "" || destination == "" {
		_ = c.manager.Fail(c.reservation.Control())
		return
	}
	bytes := 4*int64(unsafe.Sizeof(telemetrycapture.Route{})) + int64(len(endpoint)+len(protocol)+len(destination)) + int64(len(ordinals))*8
	if !c.ReserveCaptureBytes(bytes) {
		return
	}
	// Payload membership is already independently owned and immutable; each
	// route shares it. Budgeting each occurrence remains conservative.
	c.routes = append(c.routes, telemetrycapture.Route{
		PayloadID: payloadID, Ordinals: ordinals, Endpoint: strings.Clone(endpoint),
		Protocol: strings.Clone(protocol), Destination: strings.Clone(destination), EnqueuedAt: enqueuedAt,
	})
	c.routed[payloadID-1] = true
}

func (c *liveSeriesCapture) finish(deliveryErr error) {
	defer func() {
		if recover() != nil {
			_ = c.manager.Fail(c.reservation.Control())
		}
	}()
	defer func() {
		// Queued production payloads must not retain capture state through retries.
		for _, payload := range c.payloads {
			payload.ClearCapture()
		}
		c.source, c.current = nil, nil
		c.series, c.routes, c.payloads, c.routed = nil, nil, nil, nil
		c.reservation.Discard()
	}()
	if deliveryErr != nil || !c.exhausted {
		_ = c.manager.Fail(c.reservation.Control())
		return
	}
	for _, routed := range c.routed {
		if !routed {
			_ = c.manager.Fail(c.reservation.Control())
			return
		}
	}
	if len(c.routes) == 0 {
		return
	}
	if !c.ReserveCaptureBytes(int64(len(c.series))) {
		return
	}
	used := make([]bool, len(c.series))
	for _, route := range c.routes {
		for _, ordinal := range route.Ordinals {
			i := sort.Search(len(c.series), func(i int) bool { return c.series[i].Ordinal >= ordinal })
			if i == len(c.series) || c.series[i].Ordinal != ordinal {
				_ = c.manager.Fail(c.reservation.Control())
				return
			}
			used[i] = true
		}
	}
	count := 0
	for i, selected := range used {
		if selected {
			c.series[count] = c.series[i]
			count++
		}
	}
	clear(c.series[count:])
	c.series = c.series[:count]
	_ = c.reservation.Commit(telemetrycapture.Payload{Series: c.series, Routes: c.routes})
}
