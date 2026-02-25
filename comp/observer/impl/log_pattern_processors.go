// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// PatternLogProcessor is a log processor that clusterizes logs into patterns.
// Patterns will be sent to various pattern based anomaly detectors.

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
	"github.com/DataDog/datadog-agent/comp/observer/impl/queue"
)

const FILTER_ONLY_Q_LOG = false

var tlmAnomalyCount = atomic.Int64{}

// PatternLogProcessor is a log processor that detects patterns in logs.
// Pattern clusterers are distinct based on the tags logs have (https://datadoghq.atlassian.net/wiki/x/CQB6Ng):
// - env
// - service
// - source
// - pod_name
// - dirname
type PatternLogProcessor struct {
	ClustererPipeline *patterns.MultiThreadPipeline[*LogADTags]
	ResultChannel     chan *patterns.MultiThreadResult[*LogADTags]
	AnomalyDetectors  []LogAnomalyDetector
}

func NewPatternLogProcessor(anomalyDetectors []LogAnomalyDetector) *PatternLogProcessor {
	pipelineOutputChannel := make(chan *patterns.MultiThreadResult[*LogADTags], 4096)
	clustererPipeline := patterns.NewMultiThreadPipeline(runtime.NumCPU(), pipelineOutputChannel, false)

	p := &PatternLogProcessor{
		ClustererPipeline: clustererPipeline,
		ResultChannel:     pipelineOutputChannel,
		AnomalyDetectors:  anomalyDetectors,
	}

	for _, anomalyDetector := range p.AnomalyDetectors {
		anomalyDetector.SetProcessor(p)
		anomalyDetector.Run()
	}

	go func() {
		for result := range pipelineOutputChannel {
			if result.ClusterResult.IsNew {
				fmt.Printf("[cc] Processor: New cluster: (ID: %d, SEV: %s) %+v\n", result.ClusterResult.Cluster.ID, patterns.GetSeverityString(patterns.GetSeverity(result.ClusterResult.Cluster.Pattern)), result.ClusterResult.Signature)
				fmt.Printf("[cc] Processor: New cluster sample: %s\n", result.ClusterResult.Cluster.Samples[0])
			}
			for _, anomalyDetector := range p.AnomalyDetectors {
				anomalyDetector.Process(result.ClusterInput, result.ClusterResult)
			}
		}
	}()

	// Metrics
	celiandebug.callbacks = append(celiandebug.callbacks, func(metrics map[string]float64) {
		metrics["anomaly_count"] = float64(tlmAnomalyCount.Swap(0))

		// Number of patterns
		nPatterns := 0
		for _, clusterer := range p.ClustererPipeline.PatternClusterers {
			nPatterns += clusterer.PatternClusterer.NumClusters()
		}
		metrics["pattern_count"] = float64(nPatterns)

		// Memory
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		metrics["mem_total_alloc"] = float64(m.TotalAlloc / 1024 / 1024)
		metrics["mem_sys"] = float64(m.Sys / 1024 / 1024)
	})

	return p
}

func (p *PatternLogProcessor) Name() string {
	return "pattern_log_processor"
}

func (p *PatternLogProcessor) Process(log observer.LogView) observer.LogProcessorResult {
	// Process only q test logs
	if FILTER_ONLY_Q_LOG && !strings.Contains(string(log.GetContent()), "q-log") {
		return observer.LogProcessorResult{}
	}

	// TODO: Remove debug
	tags := ParseTags(log.GetTags())
	// fmt.Printf("Processing log[%+v]: %s\n", tags, string(log.GetContent()))
	t := strings.Builder{}
	for _, tag := range log.GetTags() {
		t.WriteString(string(tag))
		t.WriteString(",")
	}
	t.WriteString(string(log.GetContent()))

	fmt.Printf("Processing log[%s]: %s\n", t.String(), string(log.GetContent()))

	p.ClustererPipeline.Process(&patterns.TokenizerInput[*LogADTags]{Message: string(log.GetContent()), UserData: &tags})

	// Results arrive asynchronously via ResultChannel
	return observer.LogProcessorResult{}
}

// What we plug after the log processor
type LogAnomalyDetector interface {
	SetProcessor(processor *PatternLogProcessor)
	Run()
	Name() string
	// TODO: Pattern id not pattern to avoid multi threading issues
	Process(clustererInput *patterns.ClustererInput[*LogADTags], clustererResult *patterns.ClusterResult)
}

type LogAnomalyDetectionProcessInput struct {
	ClustererInput  *patterns.ClustererInput[*LogADTags]
	ClustererResult *patterns.ClusterResult
}

// --- Watchdog Anomaly Detector ---
// AD algorithm similar to Watchdog
type WatchdogLogAnomalyDetector struct {
	Processor *PatternLogProcessor
	// Like Observer.obsCh, will create virtual metrics here
	ObservationsChannel chan observation
	AnomalyChannel      chan<- observer.AnomalyOutput
	Period              time.Duration
	InputBatchMutex     sync.Mutex
	InputBatch          []*LogAnomalyDetectionProcessInput
	// How much decay we apply to the pattern rate
	Alpha             float64
	EvictionThreshold float64
	// "Rate" corresponds to the new patterns count with decay applied
	PatternRate map[int64]float64
	// TODO: We might optimize this in log N using segment trees
	// TODO: We should compress the baseline + the smooth part
	// Here is the structure of the history:
	// | History                      |
	// | Baseline | Current           |
	// | Baseline | Eval | Preprocess |
	// We compare the baseline to the eval part, the preprocess part is used to aggregate data to eval
	PreprocessLen int
	EvalLen       int
	BaselineLen   int
	// TODO: How to do eviction with that?
	History       map[int64]TimeSeriesHistory
	SnapshotIndex int
	AlertCooldown time.Duration
	LastAlerts    map[int64]time.Time
	GroupByKeys   map[int64]*LogADGroupByKey
	ZThreshold    float64
}

// We group anomalies by cluster ID and some tags
type LogADGroupByKey struct {
	ClusterID    int
	Tags         *LogADTags
	computedHash int64
}

func (key *LogADGroupByKey) Hash() int64 {
	if key.computedHash != 0 {
		return key.computedHash
	}

	hash := fnv.New64()
	binary.Write(hash, binary.LittleEndian, int32(key.ClusterID))
	hash.Write([]byte{0})
	hash.Write([]byte(key.Tags.Env))
	hash.Write([]byte{0})
	hash.Write([]byte(key.Tags.PodName))
	hash.Write([]byte{0})
	hash.Write([]byte(key.Tags.Service))
	hash.Write([]byte{0})
	hash.Write([]byte(key.Tags.Source))
	hash.Write([]byte{0})
	hash.Write([]byte(key.Tags.DirName))

	key.computedHash = int64(hash.Sum64())

	return key.computedHash
}

// Tags that are used to group anomalies
// Tags could be empty
type LogADTags struct {
	// /!\ Don't forget to update the Hash method when adding a new tag
	// TODO: Verify that we can get these tags from the observer (not present when run locally)
	Env     string
	PodName string
	Service string
	Source  string
	// TODO: Should we prefer dirname over filepath?
	DirName string
}

func ParseTags(tags []string) LogADTags {
	result := LogADTags{}
	for _, tag := range tags {
		if strings.HasPrefix(tag, "env:") {
			result.Env = strings.TrimPrefix(tag, "env:")
		} else if strings.HasPrefix(tag, "pod_name:") {
			result.PodName = strings.TrimPrefix(tag, "pod_name:")
		} else if strings.HasPrefix(tag, "service:") {
			result.Service = strings.TrimPrefix(tag, "service:")
		} else if strings.HasPrefix(tag, "source:") {
			result.Source = strings.TrimPrefix(tag, "source:")
		} else if strings.HasPrefix(tag, "dirname:") {
			result.DirName = strings.TrimPrefix(tag, "dirname:")
		}
	}

	return result
}

func (tags *LogADTags) FullTags() []string {
	res := make([]string, 0, 10)

	if tags.Env != "" {
		res = append(res, "env:"+tags.Env)
	}
	if tags.PodName != "" {
		res = append(res, "pod_name:"+tags.PodName)
	}
	if tags.Service != "" {
		res = append(res, "service:"+tags.Service)
	}
	if tags.Source != "" {
		res = append(res, "source:"+tags.Source)
	}
	if tags.DirName != "" {
		res = append(res, "dirname:"+tags.DirName)
	}

	return res
}

func NewWatchdogLogAnomalyDetector(anomalyChannel chan<- observer.AnomalyOutput, observationsChannel chan observation) *WatchdogLogAnomalyDetector {
	// TODO: Increase delay / sizes
	return &WatchdogLogAnomalyDetector{
		AnomalyChannel:      anomalyChannel,
		ObservationsChannel: observationsChannel,
		Period:              100 * time.Millisecond,
		Alpha:               0.95,
		EvictionThreshold:   0.1,
		PatternRate:         make(map[int64]float64),
		PreprocessLen:       10,
		EvalLen:             5,
		BaselineLen:         20,
		History:             make(map[int64]TimeSeriesHistory),
		SnapshotIndex:       0,
		AlertCooldown:       15 * time.Second,
		LastAlerts:          make(map[int64]time.Time),
		GroupByKeys:         make(map[int64]*LogADGroupByKey),
		ZThreshold:          3.0,
	}
}

// TODO: We need to wait for the history to be (fully?) filled before we can start comparing it
// Represents the history of pattern rate metrics
type TimeSeriesHistory struct {
	GroupByKey *LogADGroupByKey
	Preprocess *queue.Queue
	Eval       *queue.Queue
	Baseline   *queue.Queue
}

// TODO: Is it better to have this in the processor and not each anomaly detector?
func (w *WatchdogLogAnomalyDetector) Run() {
	go func() {
		ticker := time.NewTicker(w.Period)
		for {
			select {
			case <-ticker.C:
				// Swap the batch and clear it
				var batch []*LogAnomalyDetectionProcessInput
				{
					w.InputBatchMutex.Lock()
					batch = w.InputBatch
					w.InputBatch = make([]*LogAnomalyDetectionProcessInput, 0)
					w.InputBatchMutex.Unlock()
				}

				// Process whole batch
				w.ProcessBatch(batch)
			}
		}
	}()
}

func (w *WatchdogLogAnomalyDetector) Name() string {
	return "watchdog_log_anomaly_detector"
}

func (w *WatchdogLogAnomalyDetector) Process(clustererInput *patterns.ClustererInput[*LogADTags], clustererResult *patterns.ClusterResult) {
	w.InputBatchMutex.Lock()
	defer w.InputBatchMutex.Unlock()
	w.InputBatch = append(w.InputBatch, &LogAnomalyDetectionProcessInput{ClustererInput: clustererInput, ClustererResult: clustererResult})
}

func (w *WatchdogLogAnomalyDetector) SetProcessor(processor *PatternLogProcessor) {
	w.Processor = processor
}

func (w *WatchdogLogAnomalyDetector) ProcessBatch(batch []*LogAnomalyDetectionProcessInput) {
	// fmt.Printf("[cc] WatchdogLogAnomalyDetector: Processing batch: %d\n", len(batch))
	for _, input := range batch {
		patternRateKey := &LogADGroupByKey{ClusterID: input.ClustererResult.Cluster.ID, Tags: input.ClustererInput.UserData}
		hash := patternRateKey.Hash()
		if _, ok := w.PatternRate[hash]; !ok {
			w.PatternRate[hash] = 0
			w.GroupByKeys[hash] = patternRateKey
		}
		w.PatternRate[hash]++
	}

	// We can do anomaly detection
	if w.Snapshot() {
		/*
			if len(w.PatternRate) > 0 {
				maxRate := 0.0
				maxRatePatternID := 0
				for patternID, rate := range w.PatternRate {
					w.PatternRate[patternID] = rate * w.Alpha

					if rate < w.EvictionThreshold {
						delete(w.PatternRate, patternID)
						// fmt.Printf("[cc] WatchdogLogAnomalyDetector: Evicting pattern: %d\n", patternID)
					}

					if rate > maxRate {
						maxRate = rate
						maxRatePatternID = patternID
					}
				}

				clusterInfo, err := w.Processor.ClustererPipeline.GetClusterInfo(maxRatePatternID)
				if err != nil {
					fmt.Printf("[cc] WatchdogLogAnomalyDetector: Error getting cluster info: %v\n", err)
					return
				}

				fmt.Printf("[cc] WatchdogLogAnomalyDetector: Max Rate Cluster: Rate: %f, ID: %d, Pattern: %s\n", maxRate, maxRatePatternID, clusterInfo.PatternString)
				fmt.Printf("[cc] WatchdogLogAnomalyDetector: Pattern rates: %d\n", len(w.PatternRate))
			}
		*/

		// Send back to the observer creating virtual metrics
		for hash, rate := range w.PatternRate {
			groupByKey := w.GroupByKeys[hash]
			// TODO: Is it better to use a single metric and add a specific cluster id tag?
			// Cluster IDs are unique even though they are not on the same CPU
			virtualMetricName := fmt.Sprintf("_.watchdog_log_ad.cluster.%d", groupByKey.ClusterID)
			w.ObservationsChannel <- observation{
				source: "watchdog_log_ad",
				metric: &metricObs{
					name:  virtualMetricName,
					value: rate,
					tags:  groupByKey.Tags.FullTags(),
				},
			}
		}

		// TODO
		// Test anomaly detection
		w.DetectAnomalies()
	}
}

// Will update the history with the current pattern rate metrics
// Returns whether we updated the eval / baseline part of the history (=> preprocess buffer is full)
func (w *WatchdogLogAnomalyDetector) Snapshot() bool {
	for hash, rate := range w.PatternRate {
		if _, ok := w.History[hash]; !ok {
			w.History[hash] = TimeSeriesHistory{GroupByKey: w.GroupByKeys[hash], Preprocess: queue.NewQueue(), Eval: queue.NewQueue(), Baseline: queue.NewQueue()}
		}
		w.History[hash].Preprocess.Enqueue(rate)
	}
	w.SnapshotIndex++
	if w.SnapshotIndex >= w.PreprocessLen {
		w.SnapshotIndex = 0

		// Update eval and baseline with the new preprocess data
		for _, history := range w.History {
			// Do a simple average
			preprocessData := 0.0
			nItem := 0
			for {
				value, ok := history.Preprocess.Dequeue()
				if !ok {
					break
				}
				preprocessData += value
				nItem++
			}
			preprocessData /= float64(nItem)

			history.Eval.Enqueue(preprocessData)
			if history.Eval.Len() > w.EvalLen {
				value, ok := history.Eval.Dequeue()
				if !ok {
					// TODO: We should panic
					break
				}
				history.Baseline.Enqueue(value)
			}

			if history.Baseline.Len() > w.BaselineLen {
				_, ok := history.Baseline.Dequeue()
				if !ok {
					// TODO: We should panic
					break
				}
			}
		}

		return true
	}

	return false
}

// DetectAnomalies detects anomalies in the history
func (w *WatchdogLogAnomalyDetector) DetectAnomalies() {
	for _, history := range w.History {
		// TODO: Is half filled enough?
		if history.Baseline.Len() < w.BaselineLen/2 || history.Eval.Len() < w.EvalLen {
			continue
		}

		// fmt.Printf("[cc] WatchdogLogAnomalyDetector: Detecting anomalies for cluster: %d\n", clusterID)

		// Simple z-score
		baselineMean := 0.0
		baselineStddev := 0.0
		baselineSlice := history.Baseline.Slice()
		for _, val := range baselineSlice {
			baselineMean += val
		}
		baselineMean /= float64(len(baselineSlice))
		for _, val := range baselineSlice {
			baselineStddev += (val - baselineMean) * (val - baselineMean)
		}
		baselineStddev /= float64(len(baselineSlice))
		baselineStddev = math.Sqrt(baselineStddev)
		baselineStddev = math.Max(baselineStddev, 1e-6)

		// TODO: What to do with the eval part? Not useful for this method
		// TODO: Directly send the rates to metrics AD?
		evalMean := 0.0
		evalSlice := history.Eval.Slice()
		for _, val := range evalSlice {
			evalMean += val
		}
		evalMean /= float64(len(evalSlice))

		zScore := (evalMean - baselineMean) / baselineStddev
		// TODO: Could be +Inf on some cases
		if math.Abs(zScore) > w.ZThreshold {
			// TODO: Lock...
			w.OnAnomalyDetected(time.Now(), history.GroupByKey, zScore)
		}
	}
}

func (w *WatchdogLogAnomalyDetector) OnAnomalyDetected(date time.Time, groupByKey *LogADGroupByKey, zScore float64) {
	tlmAnomalyCount.Add(1)
	if _, ok := w.LastAlerts[groupByKey.Hash()]; !ok || time.Since(w.LastAlerts[groupByKey.Hash()]) >= w.AlertCooldown {
		w.LastAlerts[groupByKey.Hash()] = time.Now()

		clusterInfo, err := w.Processor.ClustererPipeline.GetClusterInfo(groupByKey.ClusterID)
		if err != nil {
			fmt.Printf("[cc] WatchdogLogAnomalyDetector: Error getting cluster info: %v\n", err)
			return
		}

		w.AnomalyChannel <- w.MakeAlert(date, groupByKey, zScore, &clusterInfo)
	}
}

func (w *WatchdogLogAnomalyDetector) MakeAlert(date time.Time, groupByKey *LogADGroupByKey, zScore float64, clusterInfo *patterns.ClusterInfo) observer.AnomalyOutput {
	fullTags := groupByKey.Tags.FullTags()
	title := fmt.Sprintf("Log pattern anomaly on %s for pattern: %s", fullTags, clusterInfo.PatternString)
	description := fmt.Sprintf("%s\nTags: %s\nZ-Score: %f", title, fullTags, zScore)

	return observer.AnomalyOutput{
		Source:       observer.MetricName(fmt.Sprintf("log.patterns.%s", w.Name())),
		AnalyzerName: w.Name(),
		// TODO: Type: log (or full source: log)
		// TODO: This should be removed
		SourceSeriesID: "",
		Title:          title,
		Description:    description,
		Tags:           fullTags,
		Timestamp:      date.Unix(),
		// TODO: Precise range
		TimeRange: observer.TimeRange{
			Start: date.Unix(),
			End:   date.Unix(),
		},
		// TODO: User data... (to be done after refactoring anomalies)
	}
}
