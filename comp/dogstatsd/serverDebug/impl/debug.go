// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverdebugimpl implements a component to run the dogstatsd server debug
package serverdebugimpl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"go.uber.org/atomic"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logComponentImpl "github.com/DataDog/datadog-agent/comp/core/log/impl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/identity"
	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// Requires defines the dependencies for the serverDebug component.
type Requires struct {
	compdef.In

	Log    log.Component
	Config configComponent.Component
}

// Provides defines the output of the serverDebug component.
type Provides struct {
	compdef.Out

	Comp serverdebug.Component
}

// NewComponent creates a new instance of the serverDebug component.
func NewComponent(deps Requires) Provides {
	return Provides{Comp: newServerDebugCompat(deps.Log, deps.Config)}
}

// metricStat holds how many times a metric has been
// processed and when was the last time.
type metricStat struct {
	Name     string    `json:"name"`
	Count    uint64    `json:"count"`
	LastSeen time.Time `json:"last_seen"`
	Tags     string    `json:"tags"`
}

type serverDebugImpl struct {
	stateLock sync.Mutex
	log       log.Component
	enabled   *atomic.Bool

	view          *debugStatsView
	metricsCounts metricsCountBuckets

	// clock is used to keep a consistent time state within the debug server whether
	// we use a real clock in production code or a mock clock for unit testing
	clock clock.Clock

	identityBuilders sync.Pool
	debugLoopStop    chan struct{}

	// dogstatsdDebugLogger is an instance of the logger config that can be used to create new logger for dogstatsd-stats metrics
	dogstatsdDebugLogger pkglog.LoggerInterface
}

// NewServerlessServerDebug creates a new instance of serverDebug.Component
func NewServerlessServerDebug(cfg model.Reader) serverdebug.Component {
	return newServerDebugCompat(logComponentImpl.NewTemporaryLoggerWithoutInit(), cfg)
}

func newServerDebugCompat(l log.Component, cfg model.Reader) serverdebug.Component {
	sd := &serverDebugImpl{
		log:           l,
		enabled:       atomic.NewBool(false),
		view:          newDefaultDebugStatsView(),
		metricsCounts: newMetricsCountBuckets(defaultDebugStatsShardCount),
		clock:         clock.New(),
		identityBuilders: sync.Pool{
			New: func() interface{} { return identity.NewBuilder() },
		},
	}
	sd.dogstatsdDebugLogger = sd.getDogstatsdDebug(cfg)

	return sd
}

// FormatDebugStats returns a printable version of debug stats.
func FormatDebugStats(stats []byte) (string, error) {
	var dogStats map[uint64]metricStat
	if err := json.Unmarshal(stats, &dogStats); err != nil {
		return "", err
	}

	// put metrics in order: first is the more frequent
	order := make([]uint64, len(dogStats))
	i := 0
	for metric := range dogStats {
		order[i] = metric
		i++
	}

	sort.Slice(order, func(i, j int) bool {
		return dogStats[order[i]].Count > dogStats[order[j]].Count
	})

	// write the response
	buf := bytes.NewBuffer(nil)

	header := fmt.Sprintf("%-40s | %-20s | %-10s | %-20s\n", "Metric", "Tags", "Count", "Last Seen")
	buf.Write([]byte(header))
	buf.Write([]byte(strings.Repeat("-", 40) + "-|-" + strings.Repeat("-", 20) + "-|-" + strings.Repeat("-", 10) + "-|-" + strings.Repeat("-", 20) + "\n"))

	for _, key := range order {
		stats := dogStats[key]
		buf.Write([]byte(fmt.Sprintf("%-40s | %-20s | %-10d | %-20v\n", stats.Name, stats.Tags, stats.Count, stats.LastSeen)))
	}

	if len(dogStats) == 0 {
		buf.Write([]byte("No metrics processed yet."))
	}

	return buf.String(), nil
}

// storeMetricStats stores stats on the given metric sample.
//
// It can help troubleshooting clients with bad behaviors.
func (d *serverDebugImpl) StoreMetricStats(sample metrics.MetricSample) {
	if !d.enabled.Load() {
		return
	}

	builder := d.identityBuilders.Get().(*identity.Builder)
	debugViewKey := builder.DebugView(sample)
	d.identityBuilders.Put(builder)

	if !d.enabled.Load() {
		// the debug server might have been disabled since the previous check
		return
	}

	d.storeMetricStatsWithDebugViewKey(d.clock.Now(), debugViewKey)
}

// StoreMetricStatsWithDebugViewKey stores stats using the serverDebug view key
// already computed by the worker hot path.
func (d *serverDebugImpl) StoreMetricStatsWithDebugViewKey(_ metrics.MetricSample, debugViewKey identity.DebugViewKey) {
	if !d.enabled.Load() {
		return
	}

	now := d.clock.Now()
	if !d.enabled.Load() {
		// the debug server might have been disabled since the previous check
		return
	}

	d.storeMetricStatsWithDebugViewKey(now, debugViewKey)
}

func (d *serverDebugImpl) storeMetricStatsWithDebugViewKey(now time.Time, debugViewKey identity.DebugViewKey) {
	stat := d.view.store(now, debugViewKey)
	d.metricsCounts.record(debugViewKey.Key, now)

	if d.dogstatsdDebugLogger != nil {
		logMessage := "Metric Name: %v | Tags: {%v} | Count: %v | Last Seen: %v "
		d.dogstatsdDebugLogger.Infof(logMessage, stat.Name, stat.Tags, stat.Count, stat.LastSeen)
	}
}

// SetMetricStatsEnabled enables or disables metric stats
func (d *serverDebugImpl) SetMetricStatsEnabled(enable bool) {
	d.stateLock.Lock()
	defer d.stateLock.Unlock()

	if enable {
		d.enableMetricsStats()
	} else {
		d.disableMetricsStats()
	}
}

// enableMetricsStats enables the debug mode of the DogStatsD server and start
// the debug mainloop collecting the amount of metrics received.
func (d *serverDebugImpl) enableMetricsStats() {
	// already enabled?
	if d.enabled.Load() {
		return
	}

	d.enabled.Store(true)
	stop := make(chan struct{})
	d.debugLoopStop = stop
	go d.runDebugLoop(stop)
}

func (d *serverDebugImpl) runDebugLoop(stop <-chan struct{}) {
	ticker := d.clock.Ticker(time.Millisecond * 100)
	d.log.Debug("Starting the DogStatsD debug loop.")
	defer func() {
		d.log.Debug("Stopping the DogStatsD debug loop.")
		ticker.Stop()
	}()

	var lastSpikeCheck time.Time
	for {
		select {
		case <-ticker.C:
			sec := d.clock.Now().Truncate(time.Second).Add(-time.Second)
			if sec.After(lastSpikeCheck) {
				lastSpikeCheck = sec
				if d.metricsCounts.hasSpikeAt(sec) {
					d.log.Warnf("A burst of metrics has been detected by DogStatSd: here is the last 5 seconds count of metrics: %v", d.metricsCounts.countsEndingAt(sec))
				}
			}
		case <-stop:
			return
		}
	}
}

func (d *serverDebugImpl) hasSpike() bool {
	return d.metricsCounts.hasSpikeAt(d.clock.Now())
}

// GetJSONDebugStats returns jsonified debug statistics.
func (d *serverDebugImpl) GetJSONDebugStats() ([]byte, error) {
	return json.Marshal(d.view.snapshot(d.clock.Now()))
}

func (d *serverDebugImpl) IsDebugEnabled() bool {
	return d.enabled.Load()
}

// disableMetricsStats disables the debug mode of the DogStatsD server and
// stops the debug mainloop.
func (d *serverDebugImpl) disableMetricsStats() {
	if d.enabled.Load() {
		d.enabled.Store(false)
		if d.debugLoopStop != nil {
			close(d.debugLoopStop)
			d.debugLoopStop = nil
		}
	}

	d.log.Info("Disabling DogStatsD debug metrics stats.")
}

// build a local dogstatsd logger and bubbling up any errors
func (d *serverDebugImpl) getDogstatsdDebug(cfg model.Reader) pkglog.LoggerInterface {
	if cfg == nil {
		return nil
	}

	var dogstatsdLogger pkglog.LoggerInterface

	// Configuring the log file path
	logFile := cfg.GetString("dogstatsd_log_file")
	if logFile == "" {
		logFile = defaultpaths.DogstatsDLogFile
	}

	// Set up dogstatsdLogger
	if cfg.GetBool("dogstatsd_logging_enabled") {
		logger, e := pkglogsetup.SetupDogstatsdLogger(logFile, cfg)
		if e != nil {
			// use component logger instead of global logger.
			d.log.Errorf("Unable to set up Dogstatsd logger: %v. || Please reach out to Datadog support at https://docs.datadoghq.com/help/ ", e)
			return nil
		}
		dogstatsdLogger = logger
	}
	return dogstatsdLogger

}
