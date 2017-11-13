// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package dogstatsd

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// Schema of a dogstatsd packet: see http://docs.datadoghq.com

// MetricTypes maps the dogstatsd metric types to the agent metric types
var metricTypes = map[string]metrics.MetricType{
	"g":  metrics.GaugeType,
	"c":  metrics.CounterType,
	"s":  metrics.SetType,
	"h":  metrics.HistogramType,
	"ms": metrics.HistogramType,
	"d":  metrics.DistributionType,
	"dk": metrics.DistributionKType,
	"dc": metrics.DistributionCType,
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

// parseTags parses `rawTags` and returns a slice of tags and the value of the `host:` tag if found
func parseTags(rawTags []byte, extractHost bool) ([]string, string) {
	var host string
	tags := bytes.Split(rawTags[1:], []byte(","))
	tagsList := make([]string, 0, len(tags))

	for _, tag := range tags {
		if extractHost && bytes.HasPrefix(tag, []byte("host:")) {
			host = string(tag[5:])
		} else {
			tagsList = append(tagsList, string(tag))
		}
	}
	return tagsList, host
}

func parseServiceCheckPacket(packet []byte) (*metrics.ServiceCheck, error) {
	// _sc|name|status|[metadata|...]

	splitPacket := bytes.Split(packet, []byte("|"))

	if len(splitPacket) < 3 {
		return nil, fmt.Errorf("Invalid packet format")
	}

	rawName, rawStatus := splitPacket[1], splitPacket[2]

	if len(rawName) == 0 || len(rawStatus) == 0 {
		return nil, fmt.Errorf("Invalid ServiceCheck packet format: empty 'name' or 'status' field")
	}

	service := metrics.ServiceCheck{
		CheckName: string(rawName),
	}

	if status, err := strconv.Atoi(string(rawStatus)); err != nil {
		return nil, fmt.Errorf("dogstatsd: service check has invalid 'status': %s", err)
	} else if serviceStatus, err := metrics.GetServiceCheckStatus(status); err != nil {
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
					log.Warnf("skipping timestamp: %s", err)
					continue
				}
				service.Ts = ts
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("h:")) {
				service.Host = string(rawMetadataFields[i][2:])
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("#")) {
				service.Tags, _ = parseTags(rawMetadataFields[i], false)
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("m:")) {
				service.Message = string(rawMetadataFields[i][2:])
			} else {
				log.Warnf("unknown metadata type: '%s'", rawMetadataFields[i])
			}
		}
	}

	return &service, nil
}

func parseEventPacket(packet []byte) (*metrics.Event, error) {
	// _e{title.length,text.length}:title|text
	//  [
	//   |d:date_happened
	//   |p:priority
	//   |h:hostname
	//   |t:alert_type
	//   |s:source_type_nam
	//   |#tag1,tag2
	//  ]

	packetRaw := bytes.SplitN(packet, []byte(":"), 2)
	if len(packetRaw) < 2 || len(packetRaw[0]) < 7 || len(packetRaw[1]) < 3 {
		return nil, fmt.Errorf("Invalid packet format")
	}
	header := packetRaw[0]
	packet = packetRaw[1]

	rawLen := bytes.SplitN(header[3:], []byte(","), 2)
	if len(rawLen) != 2 {
		return nil, fmt.Errorf("Invalid packet format")
	}

	titleLen, err := strconv.ParseInt(string(rawLen[0]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("Invalid packet format, could not parse title.length: '%s'", rawLen[0])
	}

	textLen, err := strconv.ParseInt(string(rawLen[1][:len(rawLen[1])-1]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("Invalid packet format, could not parse text.length: '%s'", rawLen[0])
	}
	if titleLen+textLen+1 > int64(len(packet)) {
		return nil, fmt.Errorf("Invalid packet format, title.length and text.length exceed total message length")
	}

	rawTitle := packet[:titleLen]
	rawText := packet[titleLen+1 : titleLen+1+textLen]
	packet = packet[titleLen+1+textLen:]

	if len(rawTitle) == 0 || len(rawText) == 0 {
		return nil, fmt.Errorf("Invalid event packet format: empty 'title' or 'text' field")
	}

	event := metrics.Event{
		Priority:  metrics.EventPriorityNormal,
		AlertType: metrics.EventAlertTypeInfo,
		Title:     string(rawTitle),
		Text:      string(bytes.Replace(rawText, []byte("\\n"), []byte("\n"), -1)),
	}

	// Metadata
	if len(packet) > 1 {
		rawMetadataFields := bytes.Split(packet[1:], []byte("|"))

		for i := range rawMetadataFields {
			if bytes.HasPrefix(rawMetadataFields[i], []byte("d:")) {
				ts, err := strconv.ParseInt(string(rawMetadataFields[i][2:]), 10, 64)
				if err != nil {
					log.Warnf("skipping timestamp: %s", err)
					continue
				}
				event.Ts = ts
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("p:")) {
				priority, err := metrics.GetEventPriorityFromString(string(rawMetadataFields[i][2:]))
				if err != nil {
					log.Warnf("skipping priority: %s", err)
					continue
				}
				event.Priority = priority
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("h:")) {
				event.Host = string(rawMetadataFields[i][2:])
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("t:")) {
				t, err := metrics.GetAlertTypeFromString(string(rawMetadataFields[i][2:]))
				if err != nil {
					log.Warnf("skipping alert type: %s", err)
					continue
				}
				event.AlertType = t
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("k:")) {
				event.AggregationKey = string(rawMetadataFields[i][2:])
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("s:")) {
				event.SourceTypeName = string(rawMetadataFields[i][2:])
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("#")) {
				event.Tags, _ = parseTags(rawMetadataFields[i], false)
			} else {
				log.Warnf("unknown metadata type: '%s'", rawMetadataFields[i])
			}
		}
	}

	return &event, nil
}

func parseMetricPacket(packet []byte) (*metrics.MetricSample, error) {
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
	if len(rawName) == 0 || len(rawValue) == 0 || len(rawType) == 0 {
		return nil, fmt.Errorf("Invalid metric packet format: empty 'name', 'value' or 'text' field")
	}

	// Metadata
	var metricTags []string
	var host string
	rawSampleRate := []byte("1")
	if len(splitPacket) > 2 {
		rawMetadataFields := splitPacket[2:]

		for i := range rawMetadataFields {
			if len(rawMetadataFields[i]) < 2 {
				continue
			}

			if bytes.HasPrefix(rawMetadataFields[i], []byte("#")) {
				metricTags, host = parseTags(rawMetadataFields[i], true)
			} else if bytes.HasPrefix(rawMetadataFields[i], []byte("@")) {
				rawSampleRate = rawMetadataFields[i][1:]
			} else {
				log.Warnf("unknown metadata type: '%s'", rawMetadataFields[i])
			}
		}
	}

	// Casting
	metricName := string(rawName)

	var metricType metrics.MetricType
	var ok bool
	if metricType, ok = metricTypes[string(rawType)]; !ok {
		return nil, errors.New("Invalid metric type")
	}

	metricSampleRate, err := strconv.ParseFloat(string(rawSampleRate), 64)
	if err != nil {
		return nil, errors.New("Invalid sample rate value")
	}

	sample := &metrics.MetricSample{
		Name:       metricName,
		Mtype:      metricType,
		Tags:       metricTags,
		Host:       host,
		SampleRate: metricSampleRate,
		Timestamp:  0,
	}

	if metricType == metrics.SetType {
		sample.RawValue = string(rawValue)
	} else {
		metricValue, err := strconv.ParseFloat(string(rawValue), 64)
		if err != nil {
			return nil, errors.New("Invalid metric value")
		}
		sample.RawValue = string(rawValue)
		sample.Value = metricValue
	}

	return sample, nil
}
