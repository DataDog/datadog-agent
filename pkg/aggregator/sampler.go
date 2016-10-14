package aggregator

import (
	"sort"
	"strings"
)

// Metrics stores all the metrics by context key
type Metrics struct {
	gauges   map[string]*Gauge
	rates    map[string]*Rate
	counters map[string]*Counter
}

func newMetrics() *Metrics {
	return &Metrics{
		make(map[string]*Gauge),
		make(map[string]*Rate),
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

// Serie holds a timeserie (w/ json serialization to DD API format)
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
	interval           int64
	contexts           map[string]Context // TODO: this map grows constantly, we need to flush old contexts from time to time
	metricsByTimestamp map[int64]*Metrics
}

// NewSampler returns a newly initialized Sampler
func NewSampler(interval int64) *Sampler {
	return &Sampler{interval, map[string]Context{}, map[int64]*Metrics{}}
}

func generateContextKey(metricSample *MetricSample) string {
	var contextFields []string

	sort.Strings(*(metricSample.Tags))
	contextFields = append(contextFields, *(metricSample.Tags)...)
	contextFields = append(contextFields, metricSample.Name)

	return strings.Join(contextFields, ",")
}

func (s *Sampler) calculateBucketStart(timestamp int64) int64 {
	return timestamp - timestamp%s.interval
}

// Add the metricSample to the correct bucket
func (s *Sampler) addSample(metricSample *MetricSample, timestamp int64) {
	// Keep track of the context
	contextKey := generateContextKey(metricSample)
	if _, ok := s.contexts[contextKey]; !ok {
		s.contexts[contextKey] = Context{
			Name:       metricSample.Name,
			Tags:       metricSample.Tags,
			Host:       "",
			DeviceName: "",
		}
	}

	bucketStart := s.calculateBucketStart(timestamp)
	// If it's a new bucket, initialize it
	metrics, ok := s.metricsByTimestamp[bucketStart]
	if !ok {
		metrics = newMetrics()
		s.metricsByTimestamp[bucketStart] = metrics
	}

	// Add sample to bucket
	metrics.addSample(contextKey, metricSample.Mtype, metricSample.Value, metricSample.Timestamp)
}

func (s *Sampler) flush(timestamp int64) []*Serie {
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
				// Resolve context and populate new Serie
				context := s.contexts[serie.contextKey]
				serie.Name = context.Name + serie.nameSuffix
				serie.Tags = context.Tags
				serie.Host = context.Host
				serie.DeviceName = context.DeviceName
				serie.Interval = s.interval

				serieBySignature[serieSignature] = serie
				result = append(result, serie)
			}
		}

		delete(s.metricsByTimestamp, timestamp)
	}

	return result
}

// TODO: Pass a reference to *MetricSample instead
func (m *Metrics) addSample(contextKey string, mType MetricType, value float64, timestamp int64) {
	switch mType {
	case GaugeType:
		_, ok := m.gauges[contextKey]
		if !ok {
			m.gauges[contextKey] = &Gauge{}
		}
		m.gauges[contextKey].addSample(value, timestamp)
	case CounterType:
		// pass
	case RateType:
		_, ok := m.rates[contextKey]
		if !ok {
			m.rates[contextKey] = &Rate{}
		}
		m.rates[contextKey].addSample(value, timestamp)
	}
}

func (m *Metrics) flush(timestamp int64) []*Serie {
	var series []*Serie

	// Gauges
	for contextKey, gauge := range m.gauges {
		// discard the timestamp returned here, we use the one passed to the flush
		value, _ := gauge.flush()

		serie := &Serie{
			Points:     [][]interface{}{{timestamp, value}},
			Mtype:      "gauge",
			contextKey: contextKey,
		}
		series = append(series, serie)
	}

	// Counter
	// ...

	// Rates
	for contextKey, rate := range m.rates {
		value, metricTimestamp, err := rate.flush()

		if err == nil {
			serie := &Serie{
				Points:     [][]interface{}{{metricTimestamp, value}},
				Mtype:      "gauge",
				contextKey: contextKey,
			}
			series = append(series, serie)
		}
	}

	return series
}
