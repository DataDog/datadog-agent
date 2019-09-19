package telemetry

import (
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type Counters []Counter

type Counter struct {
	Labels string // key:value,key2:value2
	Value  float64
}

// Keys returns the first dimension axis of the given counters.
//
// E.g.
// endpoint:a,route:v1,5
// endpoint:a,route:v2,50
// endpoint:b,route:v1,50
//
// Keys() -> [endpoint:a, endpoint:b]
func (c Counters) Keys() []string {
	keys := make(map[string]struct{})
	for _, cc := range c {
		key := strings.Split(cc.Labels, ",")[0] // XXX(remy): :thisisfine: 🔥
		keys[key] = struct{}{}
	}

	var rv []string
	for k := range keys {
		rv = append(rv, k)
	}

	return rv
}

// SumValues returns the sum of all values of these counters.
// TODO(remy): doc
func (c Counters) SumValues() float64 {
	var rv float64
	for _, cc := range c {
		rv += cc.Value
	}
	return rv
}

// Value returns the value of the given label.
// TODO(remy): doc
func (cs Counters) Value(label string) float64 {
	for _, c := range cs {
		if c.Labels == label {
			return c.Value
		}
	}
	return 0 // TODO(remy): err if not found?
}

func (cs Counters) KV() map[string]float64 {
	rv := make(map[string]float64)
	for _, c := range cs {
		rv[c.Labels] = c.Value
	}
	return rv
}

// SumLabels returns the sum of the given axis in the given counters.
// Warning: this function is limited to the finale dimension.
//
// E.g.
// endpoint:a,apikey:z,route:v1,1
// endpoint:b,apikey:z,route:v1,5
// endpoint:b,apikey:y,route:v2,10
// endpoint:c,apikey:z,route:v2,50
//
// SumLabels("route:v1") -> 6
// SumLabels("route:v2") -> 60
// SumLabels("apikey:z") -> 0
func (c Counters) SumLabels(label string) float64 {
	var rv float64
	for _, cc := range c {
		if strings.HasSuffix(cc.Labels, label) {
			rv += cc.Value
		}
	}
	return rv
}

// Subset returns a subset of the given counters.
//
// E.g.
// endpoint:a,route:v1,5
// endpoint:a,route:v2,50
// endpoint:b,route:v1,10
//
// Subset("endpoint:a") ->
// endpoint:a,route:v1,5
// endpoint:a,route:v2,50
func (c Counters) Subset(label string) Counters {
	var rv Counters
	for _, cc := range c {
		if strings.HasPrefix(cc.Labels, label+",") {
			rv = append(rv, Counter{
				Labels: cc.Labels[len(label)+1:],
				Value:  cc.Value,
			})
		}
	}
	return rv
}

// TODO(remy): should we cache the Gather() for a few seconds?
func GetCounters(name string) (Counters, error) {
	metrics, _ := prometheus.DefaultGatherer.Gather()
	for _, m := range metrics {
		metric := *m

		if metric.GetName() != name {
			continue
		}

		if m.GetType() == dto.MetricType_COUNTER {
			rv := make(Counters, 0)
			values := m.GetMetric()
			for _, value := range values {
				var k string
				for _, label := range value.GetLabel() {
					k += fmt.Sprintf("%s:%s,", label.GetName(), label.GetValue())
				}
				k = strings.Trim(k, ",")
				v := value.GetCounter().GetValue()
				rv = append(rv, Counter{Labels: k, Value: v})
			}
			return rv, nil
		}
	}

	return nil, fmt.Errorf("can't find counter: %s", name)
}
