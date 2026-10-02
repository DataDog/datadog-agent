// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package serializer

import (
	"strings"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// liveSeriesCapture follows one normal iterator pass. It copies selected values
// before serializer mutations and keeps them only after successful encoding.
// Fanout may accept an item more than once, but capture records it once.
type liveSeriesCapture struct {
	source      metrics.SerieSource
	current     *metrics.Serie
	manager     *telemetrycapture.Manager
	reservation *telemetrycapture.Reservation
	pending     *telemetrycapture.Series
	exhausted   bool
	series      []telemetrycapture.Series
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
	c.pending = nil
	if !c.source.MoveNext() {
		c.exhausted = true
		c.current = nil
		return false
	}
	c.current = c.source.Current()
	if v := c.current; v != nil && telemetrycapture.MetricAllowed(v.Name) {
		c.copyCurrent(v)
	}
	return true
}

func (c *liveSeriesCapture) copyCurrent(v *metrics.Serie) {
	defer c.recoverFailure()
	// Charge temporary slice growth as well as the owned copy before cloning.
	// A substring can retain a larger production buffer, so clone strings too.
	bytes := 4*int64(unsafe.Sizeof(telemetrycapture.Series{})) + int64(len(v.Name)+len(v.Host)+len(v.Device))
	bytes += int64(v.Tags.Len())*int64(unsafe.Sizeof("")) + int64(len(v.Points))*int64(unsafe.Sizeof(telemetrycapture.Point{}))
	v.Tags.ForEach(func(tag string) { bytes += int64(len(tag)) })
	if !c.reservation.Grow(bytes) {
		return
	}
	owned := &telemetrycapture.Series{
		Name: strings.Clone(v.Name), Source: uint32(v.Source),
		Type: int32(v.MType), Interval: v.Interval, Host: strings.Clone(v.Host), Device: strings.Clone(v.Device),
		Tags: make([]string, 0, v.Tags.Len()), Points: make([]telemetrycapture.Point, len(v.Points)),
	}
	v.Tags.ForEach(func(tag string) { owned.Tags = append(owned.Tags, strings.Clone(tag)) })
	for i, p := range v.Points {
		owned.Points[i] = telemetrycapture.Point{Timestamp: p.Ts, Value: p.Value}
	}
	c.pending = owned
}

func (c *liveSeriesCapture) Current() *metrics.Serie { return c.current }
func (c *liveSeriesCapture) Count() uint64           { return c.source.Count() }

func (c *liveSeriesCapture) AcceptCurrent() {
	defer c.recoverFailure()
	if c.pending != nil {
		c.series = append(c.series, *c.pending)
		c.pending = nil
	}
}

func (c *liveSeriesCapture) finish(deliveryErr error) {
	defer c.recoverFailure()
	defer func() {
		c.source, c.current, c.pending, c.series = nil, nil, nil, nil
		c.reservation.Discard()
	}()
	if deliveryErr != nil || !c.exhausted {
		_ = c.manager.Fail(c.reservation.Control())
		return
	}
	if len(c.series) != 0 {
		_ = c.reservation.Commit(telemetrycapture.Payload{Series: c.series})
	}
}

func (c *liveSeriesCapture) recoverFailure() {
	if recover() != nil {
		_ = c.manager.Fail(c.reservation.Control())
	}
}
