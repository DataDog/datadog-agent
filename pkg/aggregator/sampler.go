package aggregator

import (
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
)

// Metrics stores all the metrics by context key
type Metrics struct {
	gauges   map[string]*Gauge
	counters map[string]*Counter
}

func newMetrics() *Metrics {
	return &Metrics{
		make(map[string]*Gauge),
		make(map[string]*Counter),
	}
}

// Context holds the elements that form a context, and can be serialized into a context key
type Context struct {
	Name       string
	Tags       *[]string
	Host       string
	DeviceName string
}

// Serie holds a timeserie
type Serie struct {
	Name       string          `json:"metric"`
	Points     [][]interface{} `json:"points"`
	Tags       *[]string       `json:"tags"`
	Host       string          `json:"host"`
	DeviceName string          `json:"device_name"`
	Mtype      string          `json:"type"`
	Interval   int64           `json:"interval"`
	contextKey string
	nameSuffix string
}

// SerieSignature holds the elements that allow to know whether two similar `Serie`s
// from the same bucket can be merged into one
type SerieSignature struct {
	mType      string
	contextKey string
	nameSuffix string
}

// Sampler aggregates metrics
type Sampler struct {
	intervalSamplerByInterval map[int64]*IntervalSampler
	contexts                  map[string]Context
}

// IntervalSampler aggregates metrics with buckets of one given interval only
type IntervalSampler struct {
	interval           int64
	metricsByTimestamp map[int64]*Metrics
}

// NewSampler returns a newly initialized Sampler
func NewSampler() *Sampler {
	return &Sampler{map[int64]*IntervalSampler{}, map[string]Context{}}
}

func newIntervalSampler(interval int64) *IntervalSampler {
	return &IntervalSampler{interval, map[int64]*Metrics{}}
}

func (s *IntervalSampler) calculateBucketStart(timestamp int64) int64 {
	return timestamp - timestamp%s.interval
}

// Resolve an IntervalSampler and add the metricSample to it
func (s *Sampler) addSample(metricSample *dogstatsd.MetricSample, timestamp int64) {
	intervalSampler, ok := s.intervalSamplerByInterval[metricSample.Interval]

	if !ok {
		intervalSampler = newIntervalSampler(metricSample.Interval)
		s.intervalSamplerByInterval[metricSample.Interval] = intervalSampler
	}

	sampleContextKey := generateContextKey(metricSample)
	if _, ok := s.contexts[sampleContextKey]; !ok {
		s.contexts[sampleContextKey] = Context{
			Name:       metricSample.Name,
			Tags:       metricSample.Tags,
			Host:       "",
			DeviceName: "",
		}
	}

	intervalSampler.addSample(sampleContextKey, metricSample.Mtype, metricSample.Value, timestamp)
}

// Add a metricSample to the given IntervalSampler
func (s *IntervalSampler) addSample(contextKey string, mType dogstatsd.MetricType, value float64, timestamp int64) {
	bucketStart := s.calculateBucketStart(timestamp)
	metrics, ok := s.metricsByTimestamp[bucketStart]

	if !ok {
		metrics = newMetrics()
		s.metricsByTimestamp[bucketStart] = metrics
	}

	metrics.addSample(contextKey, mType, value)
}

func (s *Sampler) flush(timestamp int64) []*Serie {
	var result []*Serie

	for _, intervalSampler := range s.intervalSamplerByInterval {
		series := intervalSampler.flush(timestamp)

		// Resolve context
		for _, serie := range series {
			context := s.contexts[serie.contextKey]
			serie.Name = context.Name + serie.nameSuffix
			serie.Tags = context.Tags
			serie.Host = context.Host
			serie.DeviceName = context.DeviceName
			result = append(result, serie)
		}
	}

	return result
}

func (s *IntervalSampler) flush(timestamp int64) []*Serie {
	var result []*Serie
	serieBySignature := make(map[SerieSignature]*Serie)

	// Compute a limit timestamp
	cutoffTime := s.calculateBucketStart(timestamp)

	// Iter on each bucket
	for timestamp, metrics := range s.metricsByTimestamp {
		// disregard when the timestamp is too recent
		if cutoffTime <= timestamp {
			continue
		}

		series := metrics.flush(timestamp)
		for _, serie := range series {
			serieSignature := SerieSignature{serie.Mtype, serie.contextKey, serie.nameSuffix}

			if existingSerie, ok := serieBySignature[serieSignature]; ok {
				existingSerie.Points = append(existingSerie.Points, serie.Points[0])
			} else {
				serie.Interval = s.interval
				serieBySignature[serieSignature] = serie
				result = append(result, serie)
			}
		}

		delete(s.metricsByTimestamp, timestamp)
	}

	return result
}

func generateContextKey(metricSample *dogstatsd.MetricSample) string {
	var contextFields []string

	sort.Strings(*(metricSample.Tags))
	contextFields = append(contextFields, *(metricSample.Tags)...)
	contextFields = append(contextFields, metricSample.Name)

	return strings.Join(contextFields, ",")
}

// TODO: Pass a reference to *MetricSample instead
func (m *Metrics) addSample(contextKey string, mType dogstatsd.MetricType, value float64) {
	switch mType {
	case dogstatsd.Gauge:
		_, ok := m.gauges[contextKey]
		if !ok {
			m.gauges[contextKey] = &Gauge{}
		}
		m.gauges[contextKey].addSample(value)
	case dogstatsd.Counter:
		// pass
	}
}

func (m *Metrics) flush(timestamp int64) []*Serie {
	var series []*Serie

	// Gauges
	for contextKey, gauge := range m.gauges {
		value := gauge.flush()

		serie := &Serie{
			Points:     [][]interface{}{{timestamp, value}},
			Mtype:      "gauge",
			contextKey: contextKey,
		}
		series = append(series, serie)
	}

	// Counter
	// ...

	return series
}
