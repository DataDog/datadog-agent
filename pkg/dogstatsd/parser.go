package dogstatsd

import (
	"bytes"
	"errors"
	"strconv"
)

// Schema of a dogstatsd packet:
// <name>:<value>|<metric_type>|@<sample_rate>|#<tag1_name>:<tag1_value>,<tag2_name>:<tag2_value>:<value>|<metric_type>...

type MetricType string

const (
	Gauge   MetricType = "g"
	Counter MetricType = "c"
)

// Default metrics interval in seconds (== default bucket size)
const dogstatsdInterval int64 = 10

var metricTypes map[MetricType]struct{} = map[MetricType]struct{}{
	Gauge: struct{}{},
}

type MetricSample struct {
	Name       string
	Value      float64
	Mtype      MetricType
	Tags       *[]string
	SampleRate float64
	Interval   int64
}

// mtypes
//  'g': BucketGauge,
//  'c': Counter,
//  'h': Histogram,
//  'ms': Histogram,
//  's': Set,

func nextMetric(datagram *[]byte) (*MetricSample, error) {
	// call parseMetricPacket for the first line of buffer
	var packet []byte

	if len(*datagram) == 0 {
		return nil, nil
	}
	split := bytes.SplitAfterN(*datagram, []byte("\n"), 2)

	*datagram = (*datagram)[len(split[0]):]

	// Remove trailing newline
	if len(split) == 2 {
		packet = split[0][:len(split[0])-1]
	} else {
		packet = split[0]
	}

	return parseMetricPacket(packet)
}

func parseMetricPacket(packet []byte) (*MetricSample, error) {
	// daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2
	// daemon:666|g|@0.1|#sometag:somevalue"

	splitPacket := bytes.Split(packet, []byte("|"))

	if len(splitPacket) < 2 || len(splitPacket) > 4 {
		return nil, errors.New("Invalid packet format")
	}

	// Extract name, value and type
	rawNameAndValue := bytes.Split(splitPacket[0], []byte(":"))

	if len(rawNameAndValue) != 2 {
		return nil, errors.New("Invalid packet format")
	}

	rawName, rawValue, rawType := rawNameAndValue[0], rawNameAndValue[1], splitPacket[1]

	// Metadata
	rawTags := [][]byte{}
	rawSampleRate := []byte("1")
	if len(splitPacket) > 2 {
		rawMetadataFields := splitPacket[2:]

		for i := range rawMetadataFields {
			if len(rawMetadataFields[i]) < 2 {
				continue
			}

			if bytes.HasPrefix(rawMetadataFields[i], []byte("#")) {
				rawTags = bytes.Split(rawMetadataFields[i][1:], []byte(","))
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("@")) {
				rawSampleRate = rawMetadataFields[i][1:]
			} else {
				return nil, errors.New("Invalid packet format")
			}
		}
	}

	// Casting
	metricName := string(rawName)
	metricValue, err := strconv.ParseFloat(string(rawValue), 64)
	if err != nil {
		return nil, errors.New("Invalid metric value")
	}

	metricSampleRate, err := strconv.ParseFloat(string(rawSampleRate), 64)
	if err != nil {
		return nil, errors.New("Invalid sample rate value")
	}

	metricTags := make([]string, len(rawTags))

	for i := range rawTags {
		metricTags[i] = string(rawTags[i])
	}

	metricType := MetricType(rawType)
	if _, ok := metricTypes[metricType]; !ok {
		return nil, errors.New("Invalid metric type")
	}

	return &MetricSample{metricName, metricValue, metricType, &metricTags, metricSampleRate, dogstatsdInterval}, nil
}
