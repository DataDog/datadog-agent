// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package offlinereporterimpl implements the offlinereporter component.
package offlinereporterimpl

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/afero"

	demultiplexer "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/config"
	offlinereporter "github.com/DataDog/datadog-agent/comp/offlinereporter/def"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// Params allows overriding the filesystem implementation (e.g. in tests).
type Params struct {
	Fs afero.Fs
}

// NewParams returns production Params using the real OS filesystem.
func NewParams() Params {
	return Params{Fs: afero.NewOsFs()}
}

// Requires defines the dependencies for the offlinereporter component.
type Requires struct {
	Lifecycle     compdef.Lifecycle
	Config        config.Component
	Log           log.Component
	Demultiplexer demultiplexer.Component
	Hostname      hostnameinterface.Component
	Params        Params
}

// Provides defines the output of the offlinereporter component.
type Provides struct {
	Comp offlinereporter.Component
}

// sampleSender is the subset of demultiplexer.Component used by offlinereporterImpl.
// Keeping it narrow makes the component easy to test.
type sampleSender interface {
	SendSamplesWithoutAggregation(metrics.MetricSampleBatch)
}

// hostnameGetter is the subset of hostnameinterface.Component used by offlinereporterImpl.
type hostnameGetter interface {
	GetSafe(context.Context) string
}

type offlinereporterImpl struct {
	log              log.Component
	fs               afero.Fs
	filePath         string
	demux            sampleSender
	hostnameComp     hostnameGetter
	resolvedHostname string
	lastHeartbeat    time.Time
	hasLastBeat      bool
	stopChan         chan struct{}
}

// NewComponent creates a new offlinereporter component.
func NewComponent(reqs Requires) (Provides, error) {
	h := &offlinereporterImpl{
		log:          reqs.Log,
		fs:           reqs.Params.Fs,
		filePath:     filepath.Join(reqs.Config.GetString("run_path"), "agent_heartbeat"),
		demux:        reqs.Demultiplexer,
		hostnameComp: reqs.Hostname,
		stopChan:     make(chan struct{}),
	}
	if reqs.Config.GetBool("telemetry.offlinereporter.enabled") {
		reqs.Lifecycle.Append(compdef.Hook{
			OnStart: func(ctx context.Context) error { return h.onStart(ctx) },
			OnStop:  func(_ context.Context) error { h.stopChan <- struct{}{}; return nil },
		})
	}
	return Provides{Comp: h}, nil
}

func (h *offlinereporterImpl) onStart(ctx context.Context) error {
	h.resolvedHostname = h.hostnameComp.GetSafe(ctx)
	if data, err := afero.ReadFile(h.fs, h.filePath); err == nil {
		if secs, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil {
			h.lastHeartbeat = time.Unix(secs, 0)
			h.hasLastBeat = true
		} else {
			h.log.Warnf("offlinereporter: failed to parse previous timestamp in %s: %w", h.filePath, err)
		}
	} else if !os.IsNotExist(err) {
		h.log.Warnf("offlinereporter: failed to read heartbeat file %s: %w", h.filePath, err)
	}
	go h.loop()
	return nil
}

func (h *offlinereporterImpl) writeNow() error {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	return afero.WriteFile(h.fs, h.filePath, []byte(ts), 0600)
}

func (h *offlinereporterImpl) loop() {
	if err := h.writeNow(); err != nil {
		h.log.Errorf("offlinereporter: failed to write heartbeat file %s: %w", h.filePath, err)
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-h.stopChan:
			return
		case <-ticker.C:
			_ = h.writeNow()
		}
	}
}

// SendOfflineDuration sends a gauge metric representing how long the agent was
// offline between the previous run and the current startup. It is a no-op if
// no previous heartbeat file was found (first run).
func (h *offlinereporterImpl) SendOfflineDuration(metricName string, tags []string) {
	if !h.hasLastBeat {
		return
	}
	h.demux.SendSamplesWithoutAggregation(metrics.MetricSampleBatch{
		{
			Name:       metricName,
			Value:      time.Since(h.lastHeartbeat).Seconds(),
			Mtype:      metrics.GaugeType,
			Tags:       tags,
			Host:       h.resolvedHostname,
			SampleRate: 1.0,
			Timestamp:  float64(time.Now().Unix()),
		},
	})
}
