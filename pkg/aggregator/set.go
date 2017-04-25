package aggregator

// Set tracks the number of unique elements in a set. They are only use by
// DogStatsD
type Set struct {
	values map[string]bool
}

// NewSet return a new initialized Set
func NewSet() *Set {
	return &Set{values: make(map[string]bool)}
}

func (s *Set) addSample(sample *MetricSample, timestamp int64) {
	s.values[sample.RawValue] = true
}

func (s *Set) flush(timestamp int64) ([]*Serie, error) {
	if len(s.values) == 0 {
		return []*Serie{}, NoSerieError{}
	}

	res := []*Serie{
		&Serie{
			// we use the timestamp passed to the flush
			Points: []Point{{Ts: timestamp, Value: float64(len(s.values))}},
			MType:  APIGaugeType,
		},
	}

	s.values = make(map[string]bool)
	return res, nil
}
