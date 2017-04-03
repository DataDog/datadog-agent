package dogstatsd

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// Schema of a dogstatsd packet:
// <name>:<value>|<metric_type>|@<sample_rate>|#<tag1_name>:<tag1_value>,<tag2_name>:<tag2_value>:<value>|<metric_type>...

// MetricTypes maps the dogstatsd metric types to the aggregator metric types
var metricTypes = map[string]aggregator.MetricType{
	"g": aggregator.GaugeType,
	"c": aggregator.CounterType,
}

func nextPacket(datagram *[]byte) (packet []byte) {
	if len(*datagram) == 0 {
		return nil
	}
	split := bytes.SplitAfterN(*datagram, []byte("\n"), 2)

	*datagram = (*datagram)[len(split[0]):]

	// Remove trailing newline
	if len(split) == 2 {
		packet = split[0][:len(split[0])-1]
	} else {
		packet = split[0]
	}

	return packet
}

func parseServiceCheckPacket(packet []byte) (*aggregator.ServiceCheck, error) {
	// _sc|name|status|(metadata|...)

	splitPacket := bytes.Split(packet, []byte("|"))

	if len(splitPacket) < 3 {
		return nil, fmt.Errorf("Invalid packet format")
	}

	rawName, rawStatus := splitPacket[1], splitPacket[2]

	service := aggregator.ServiceCheck{
		CheckName: string(rawName),
	}

	if status, err := strconv.Atoi(string(rawStatus)); err != nil {
		return nil, fmt.Errorf("dogstatsd: service check has invalid 'status': %s", err)
	} else if serviceStatus, err := aggregator.GetServiceCheckStatus(status); err != nil {
		return nil, fmt.Errorf("dogstatsd: unknown service check 'status': %s", err)
	} else {
		service.Status = serviceStatus
	}

	// Metadata
	if len(splitPacket) > 3 {
		rawMetadataFields := splitPacket[3:]

		for i := range rawMetadataFields {
			if bytes.HasPrefix(rawMetadataFields[i], []byte("d:")) {
				ts, err := strconv.ParseInt(string(rawMetadataFields[i][2:]), 10, 64)
				if err != nil {
					return nil, fmt.Errorf("Invalid timestamp value: '%s'", err)
				}
				service.Ts = ts
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("h:")) {
				service.Host = string(rawMetadataFields[i][2:])
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("#")) {
				rawTags := bytes.Split(rawMetadataFields[i][1:], []byte(","))
				service.Tags = make([]string, len(rawTags))

				for i := range rawTags {
					service.Tags[i] = string(rawTags[i])
				}
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("m:")) {
				service.Message = string(rawMetadataFields[i][2:])
			} else {
				return nil, fmt.Errorf("unknown metadata type: '%s'", rawMetadataFields[i])
			}
		}
	}

	return &service, nil
}

func parseEventPacket(packet []byte) (*aggregator.Event, error) {
	return nil, fmt.Errorf("Not Implemented")
}

func parseMetricPacket(packet []byte) (*aggregator.MetricSample, error) {
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

	var metricType aggregator.MetricType
	var ok bool
	if metricType, ok = metricTypes[string(rawType)]; !ok {
		return nil, errors.New("Invalid metric type")
	}

	return &aggregator.MetricSample{
		Name:       metricName,
		Value:      metricValue,
		Mtype:      metricType,
		Tags:       &metricTags,
		SampleRate: metricSampleRate,
		Timestamp:  0,
	}, nil
}
