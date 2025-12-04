// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverdebugimpltopk implements a component to run the dogstatsd server debug
package serverdebugimpltopk

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
	"go.uber.org/fx"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logComponentImpl "github.com/DataDog/datadog-agent/comp/core/log/impl"
	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newServerDebug))
}

type dependencies struct {
	fx.In

	Log    log.Component
	Config configComponent.Component
}

// metricStat holds how many times a metric has been
// processed and when was the last time.
type metricStat struct {
	Name     string    `json:"name"`
	Count    uint64    `json:"count"`
	LastSeen time.Time `json:"last_seen"`
	Tags     string    `json:"tags"`
	key      ckey.ContextKey
}

// metricStatsShard holds a subset of metric stats with its own lock
// to allow concurrent access to different shards
type metricStatsShard struct {
	sync.RWMutex
	stats           map[ckey.ContextKey]metricStat
	tagsAccumulator *tagset.HashingTagsAccumulator
}

const defaultNumShards = uint32(16) // Power of 2 for efficient modulo operation

type serverDebugImpl struct {
	sync.RWMutex
	log       log.Component
	enabled   *atomic.Bool
	shards    []*metricStatsShard
	numShards uint32
	// counting number of metrics processed last X seconds
	metricsCounts metricsCountBuckets
	// keyGen is used to generate hashes of the metrics received by dogstatsd
	keyGen *ckey.KeyGenerator

	// clock is used to keep a consistent time state within the debug server whether
	// we use a real clock in production code or a mock clock for unit testing
	clock clock.Clock
	// dogstatsdDebugLogger is an instance of the logger config that can be used to create new logger for dogstatsd-stats metrics
	dogstatsdDebugLogger pkglog.LoggerInterface
}

// NewServerlessServerDebug creates a new instance of serverDebug.Component
func NewServerlessServerDebug() serverdebug.Component {
	return newServerDebugCompat(logComponentImpl.NewTemporaryLoggerWithoutInit(), pkgconfigsetup.Datadog())
}

// newServerDebug creates a new instance of a ServerDebug
func newServerDebug(deps dependencies) serverdebug.Component {
	return newServerDebugCompat(deps.Log, deps.Config)
}

func newServerDebugCompat(l log.Component, cfg model.Reader) serverdebug.Component {
	numShards := defaultNumShards
	sd := &serverDebugImpl{
		log:     l,
		enabled: atomic.NewBool(false),
		metricsCounts: metricsCountBuckets{
			counts:     [5]uint64{0, 0, 0, 0, 0},
			metricChan: make(chan struct{}),
			closeChan:  make(chan struct{}),
		},
		keyGen:    ckey.NewKeyGenerator(),
		clock:     clock.New(),
		shards:    make([]*metricStatsShard, numShards),
		numShards: numShards,
	}
	// Initialize all shards
	for i := uint32(0); i < sd.numShards; i++ {
		sd.shards[i] = &metricStatsShard{
			stats:           make(map[ckey.ContextKey]metricStat, 1),
			tagsAccumulator: tagset.NewHashingTagsAccumulator(),
		}
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

	header := fmt.Sprintf("%-40s | %-20s | %-10s | %s\n", "Metric", "Tags", "Count", "Last Seen")
	buf.Write([]byte(header))
	buf.Write([]byte(strings.Repeat("-", 40) + "-|-" + strings.Repeat("-", 20) + "-|-" + strings.Repeat("-", 10) + "-|-" + strings.Repeat("-", 20) + "\n"))

	for _, key := range order {
		dStats := dogStats[key]
		buf.Write([]byte(fmt.Sprintf("%-40s | %-20s | %-10d | %-20v\n", dStats.Name, dStats.Tags, dStats.Count, dStats.LastSeen)))
	}

	if len(dogStats) == 0 {
		buf.Write([]byte("No metrics processed yet."))
	}

	return buf.String(), nil
}

// StoreMetricStats stores stats on the given metric sample.
//
// It can help troubleshooting clients with bad behaviors.
func (d *serverDebugImpl) StoreMetricStats(sample metrics.MetricSample) {
	if !d.enabled.Load() {
		return
	}

	now := d.clock.Now()

	// Determine which shard to use based on metric name hash
	// Using a simple hash function for distribution
	hash := hashString(sample.Name)
	shardIdx := hash % d.numShards
	shard := d.shards[shardIdx]

	// Lock only the specific shard, not the entire structure
	shard.Lock()
	defer shard.Unlock()

	// Reset and populate tags accumulator for this shard
	shard.tagsAccumulator.Reset()
	shard.tagsAccumulator.Append(sample.Tags...)

	// Generate key for this metric
	key := d.keyGen.Generate(sample.Name, "", shard.tagsAccumulator)

	ms := metricStat{
		key:  key,
		Name: sample.Name,
		Tags: strings.Join(shard.tagsAccumulator.Get(), " "), // we don't want/need to share the underlying array
	}

	// Get or create metric stat
	ms, exists := shard.stats[key]
	if !exists {
		ms = metricStat{
			Name: sample.Name,
			Tags: strings.Join(shard.tagsAccumulator.Get(), " "), // we don't want/need to share the underlying array
		}
	}

	// Update stats
	ms.Count++
	ms.LastSeen = now

	// Store back to shard
	shard.stats[key] = ms

	// Log if enabled
	if d.dogstatsdDebugLogger != nil {
		logMessage := "Metric Name: %v | Tags: {%v} | Count: %v | Last Seen: %v "
		d.dogstatsdDebugLogger.Infof(logMessage, ms.Name, ms.Tags, ms.Count, ms.LastSeen)
	}

	// Notify metrics count tracker
	select {
	case d.metricsCounts.metricChan <- struct{}{}:
	default:
		// Non-blocking send to avoid deadlock if channel is full
	}
}

// hashString returns a hash value for a string
func hashString(s string) uint32 {
	h := uint32(0)
	for i := 0; i < len(s); i++ {
		h = h*31 + uint32(s[i])
	}
	return h
}

// SetMetricStatsEnabled enables or disables metric stats
func (d *serverDebugImpl) SetMetricStatsEnabled(enable bool) {
	d.Lock()
	defer d.Unlock()

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
	go func() {
		ticker := d.clock.Ticker(time.Millisecond * 100)
		d.log.Debug("Starting the DogStatsD debug loop.")
		defer func() {
			d.log.Debug("Stopping the DogStatsD debug loop.")
			ticker.Stop()
		}()
		for {
			select {
			case <-ticker.C:
				sec := d.clock.Now().Truncate(time.Second)
				if sec.After(d.metricsCounts.currentSec) {
					d.metricsCounts.currentSec = sec
					if d.hasSpike() {
						d.log.Warnf("A burst of metrics has been detected by DogStatSd: here is the last 5 seconds count of metrics: %v", d.metricsCounts.counts)
					}

					d.metricsCounts.bucketIdx++

					if d.metricsCounts.bucketIdx >= len(d.metricsCounts.counts) {
						d.metricsCounts.bucketIdx = 0
					}

					d.metricsCounts.counts[d.metricsCounts.bucketIdx] = 0
				}
			case <-d.metricsCounts.metricChan:
				d.metricsCounts.counts[d.metricsCounts.bucketIdx]++
			case <-d.metricsCounts.closeChan:
				return
			}
		}
	}()
}

func (d *serverDebugImpl) hasSpike() bool {
	// compare this one to the sum of all others
	// if the difference is higher than all others sum, consider this
	// as an anomaly.
	var sum uint64
	for _, v := range d.metricsCounts.counts {
		sum += v
	}
	sum -= d.metricsCounts.counts[d.metricsCounts.bucketIdx]

	return d.metricsCounts.counts[d.metricsCounts.bucketIdx] > sum
}

// GetJSONDebugStats returns jsonified debug statistics.
func (d *serverDebugImpl) GetJSONDebugStats() ([]byte, error) {
	// Aggregate stats from all shards
	aggregatedStats := make(map[ckey.ContextKey]metricStat)

	for i := uint32(0); i < d.numShards; i++ {
		shard := d.shards[i]
		shard.RLock()
		for key, stat := range shard.stats {
			aggregatedStats[key] = stat
		}
		shard.RUnlock()
	}

	return json.Marshal(aggregatedStats)
}

func (d *serverDebugImpl) IsDebugEnabled() bool {
	return d.enabled.Load()
}

// disableMetricsStats disables the debug mode of the DogStatsD server and
// stops the debug mainloop.

func (d *serverDebugImpl) disableMetricsStats() {
	if d.enabled.Load() {
		d.enabled.Store(false)
		d.metricsCounts.closeChan <- struct{}{}
	}

	d.log.Info("Disabling DogStatsD debug metrics stats.")
}

// build a local dogstatsd logger and bubbling up any errors
func (d *serverDebugImpl) getDogstatsdDebug(cfg model.Reader) pkglog.LoggerInterface {

	var dogstatsdLogger pkglog.LoggerInterface

	// Configuring the log file path
	logFile := cfg.GetString("dogstatsd_log_file")
	if logFile == "" {
		logFile = defaultpaths.DogstatsDLogFile
	}

	// Set up dogstatsdLogger
	if cfg.GetBool("dogstatsd_logging_enabled") {
		logger, e := pkglogsetup.SetupDogstatsdLogger(logFile, pkgconfigsetup.Datadog())
		if e != nil {
			// use component logger instead of global logger.
			d.log.Errorf("Unable to set up Dogstatsd logger: %v. || Please reach out to Datadog support at https://docs.datadoghq.com/help/ ", e)
			return nil
		}
		dogstatsdLogger = logger
	}
	return dogstatsdLogger

}
