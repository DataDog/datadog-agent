// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverdebugimpl implements a component to run the dogstatsd server debug
package serverdebugimpl

import (
	"strings"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	slog "github.com/cihub/seelog"
	"go.uber.org/atomic"
	"go.uber.org/fx"

	commonpath "github.com/DataDog/datadog-agent/cmd/agent/common/path"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	logComponentImpl "github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	panic("not called")
}

type dependencies struct {
	fx.In

	Log    logComponent.Component
	Config configComponent.Component
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
	sync.Mutex
	log     logComponent.Component
	enabled *atomic.Bool
	Stats   map[ckey.ContextKey]metricStat `json:"stats"`
	// counting number of metrics processed last X seconds
	metricsCounts metricsCountBuckets
	// keyGen is used to generate hashes of the metrics received by dogstatsd
	keyGen *ckey.KeyGenerator

	// clock is used to keep a consistent time state within the debug server whether
	// we use a real clock in production code or a mock clock for unit testing
	clock           clock.Clock
	tagsAccumulator *tagset.HashingTagsAccumulator
	// dogstatsdDebugLogger is an instance of the logger config that can be used to create new logger for dogstatsd-stats metrics
	dogstatsdDebugLogger slog.LoggerInterface
}

// NewServerlessServerDebug creates a new instance of serverDebug.Component
func NewServerlessServerDebug() serverdebug.Component {
	return newServerDebugCompat(logComponentImpl.NewTemporaryLoggerWithoutInit(), config.Datadog)
}

// newServerDebug creates a new instance of a ServerDebug
func newServerDebug(deps dependencies) serverdebug.Component {
	panic("not called")
}

func newServerDebugCompat(log logComponent.Component, cfg config.Reader) serverdebug.Component {
	sd := &serverDebugImpl{
		log:     log,
		enabled: atomic.NewBool(false),
		Stats:   make(map[ckey.ContextKey]metricStat),
		metricsCounts: metricsCountBuckets{
			counts:     [5]uint64{0, 0, 0, 0, 0},
			metricChan: make(chan struct{}),
			closeChan:  make(chan struct{}),
		},
		keyGen: ckey.NewKeyGenerator(),
		clock:  clock.New(),
	}
	sd.dogstatsdDebugLogger = sd.getDogstatsdDebug(cfg)

	return sd
}

// metricsCountBuckets is counting the amount of metrics received for the last 5 seconds.
// It is used to detect spikes.
type metricsCountBuckets struct {
	counts     [5]uint64
	bucketIdx  int
	currentSec time.Time
	metricChan chan struct{}
	closeChan  chan struct{}
}

// FormatDebugStats returns a printable version of debug stats.
func FormatDebugStats(stats []byte) (string, error) {
	panic("not called")
}

// storeMetricStats stores stats on the given metric sample.
//
// It can help troubleshooting clients with bad behaviors.
func (d *serverDebugImpl) StoreMetricStats(sample metrics.MetricSample) {
	if !d.enabled.Load() {
		return
	}

	now := d.clock.Now()
	d.Lock()
	defer d.Unlock()

	if d.tagsAccumulator == nil {
		d.tagsAccumulator = tagset.NewHashingTagsAccumulator()
	}

	// key
	defer d.tagsAccumulator.Reset()
	d.tagsAccumulator.Append(sample.Tags...)
	key := d.keyGen.Generate(sample.Name, "", d.tagsAccumulator)

	// store
	ms := d.Stats[key]
	ms.Count++
	ms.LastSeen = now
	ms.Name = sample.Name
	ms.Tags = strings.Join(d.tagsAccumulator.Get(), " ") // we don't want/need to share the underlying array
	d.Stats[key] = ms

	if d.dogstatsdDebugLogger != nil {
		logMessage := "Metric Name: %v | Tags: {%v} | Count: %v | Last Seen: %v "
		d.dogstatsdDebugLogger.Infof(logMessage, ms.Name, ms.Tags, ms.Count, ms.LastSeen)
	}

	d.metricsCounts.metricChan <- struct{}{}
}

// SetMetricStatsEnabled enables or disables metric stats
func (d *serverDebugImpl) SetMetricStatsEnabled(enable bool) {
	panic("not called")
}

// enableMetricsStats enables the debug mode of the DogStatsD server and start
// the debug mainloop collecting the amount of metrics received.
func (d *serverDebugImpl) enableMetricsStats() {
	panic("not called")
}

func (d *serverDebugImpl) hasSpike() bool {
	panic("not called")
}

// GetJSONDebugStats returns jsonified debug statistics.
func (d *serverDebugImpl) GetJSONDebugStats() ([]byte, error) {
	panic("not called")
}

func (d *serverDebugImpl) IsDebugEnabled() bool {
	panic("not called")
}

// disableMetricsStats disables the debug mode of the DogStatsD server and
// stops the debug mainloop.

func (d *serverDebugImpl) disableMetricsStats() {
	panic("not called")
}

// build a local dogstatsd logger and bubbling up any errors
func (d *serverDebugImpl) getDogstatsdDebug(cfg config.Reader) slog.LoggerInterface {

	var dogstatsdLogger slog.LoggerInterface

	// Configuring the log file path
	logFile := cfg.GetString("dogstatsd_log_file")
	if logFile == "" {
		logFile = commonpath.DefaultDogstatsDLogFile
	}

	// Set up dogstatsdLogger
	if cfg.GetBool("dogstatsd_logging_enabled") {
		logger, e := config.SetupDogstatsdLogger(logFile)
		if e != nil {
			// use component logger instead of global logger.
			d.log.Errorf("Unable to set up Dogstatsd logger: %v. || Please reach out to Datadog support at https://docs.datadoghq.com/help/ ", e)
			return nil
		}
		dogstatsdLogger = logger
	}
	return dogstatsdLogger

}
