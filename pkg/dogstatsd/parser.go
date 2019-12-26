// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Schema of a dogstatsd packet: see http://docs.datadoghq.com

type tagRetriever func(entity string, cardinality collectors.TagCardinality) ([]string, error)

var (
	// metricTypes maps the dogstatsd metric types to the agent metric types
	metricTypes = map[string]metrics.MetricType{
		"g":  metrics.GaugeType,
		"c":  metrics.CounterType,
		"s":  metrics.SetType,
		"h":  metrics.HistogramType,
		"ms": metrics.HistogramType,
		"d":  metrics.DistributionType,
	}
	tagSeparator                      = []byte(",")
	fieldSeparator                    = []byte("|")
	valueSeparator                    = []byte(":")
	hostTagPrefix                     = []byte("host:")
	entityIDTagPrefix                 = []byte("dd.internal.entity_id:")
	lenHostTagPrefix                  = len(hostTagPrefix)
	lenEntityIDTagPrefix              = len(entityIDTagPrefix)
	getTags              tagRetriever = tagger.Tag
)

func nextMessage(packet *[]byte) (message []byte) {
	if len(*packet) == 0 {
		return nil
	}

	advance, message, err := bufio.ScanLines(*packet, true)
	if err != nil || len(message) == 0 {
		return nil
	}

	*packet = (*packet)[advance:]
	return message
}

// nextField returns the data found before the given separator and
// the remainder, as a no-heap alternative to bytes.Split.
// If the separator is not found, the remainder is nil.
func nextField(slice, sep []byte) ([]byte, []byte) {
	sepIndex := bytes.Index(slice, sep)
	if sepIndex == -1 {
		return slice, nil
	}
	return slice[:sepIndex], slice[sepIndex+1:]
}

// parseTags parses `rawTags` and returns a slice of tags
// and the extracted hostname, injects tagger tags if an entity
// is provided via a special tag
func parseTags(rawTags []byte, defaultHostname string) ([]string, string) {
	if len(rawTags) == 0 {
		return nil, defaultHostname
	}

	tagsList := make([]string, 0, bytes.Count(rawTags, tagSeparator)+1)
	host := defaultHostname
	remainder := rawTags

	var tag []byte
	for {
		tag, remainder = nextField(remainder, tagSeparator)
		if bytes.HasPrefix(tag, hostTagPrefix) {
			host = string(tag[lenHostTagPrefix:])
		} else if bytes.HasPrefix(tag, entityIDTagPrefix) {
			// currently only supported for pods
			entity := kubelet.KubePodTaggerEntityPrefix + string(tag[lenEntityIDTagPrefix:])
			entityTags, err := getTags(entity, tagger.DogstatsdCardinality)
			if err != nil {
				log.Tracef("Cannot get tags for entity %s: %s", entity, err)
				continue
			}
			tagsList = append(tagsList, entityTags...)
		} else {
			tagsList = append(tagsList, string(tag))
		}

		if remainder == nil {
			break
		}
	}
	return tagsList, host
}

func parseServiceCheckMessage(message []byte, defaultHostname string) (*metrics.ServiceCheck, error) {
	// _sc|name|status|[metadata|...]

	separatorCount := bytes.Count(message, fieldSeparator)
	if separatorCount < 2 {
		return nil, fmt.Errorf("invalid field number for %q", message)
	}
	rawName, remainder := nextField(message[4:], fieldSeparator)
	rawStatus, remainder := nextField(remainder, fieldSeparator)

	if len(rawName) == 0 || len(rawStatus) == 0 {
		return nil, fmt.Errorf("Invalid ServiceCheck message format: empty 'name' or 'status' field")
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

	// Handle hostname, with a priority to the h: field, then the host:
	// tag and finally the defaultHostname value
	var hostFromField string
	hostFromTags := defaultHostname

	// Metadata
	for {
		var rawMetadataField []byte
		rawMetadataField, remainder = nextField(remainder, fieldSeparator)
		if rawMetadataField == nil {
			break
		}

		if bytes.HasPrefix(rawMetadataField, []byte("d:")) {
			ts, err := parseInt64(rawMetadataField[2:])
			if err != nil {
				log.Warnf("skipping timestamp: %s", err)
				continue
			}
			service.Ts = ts
		} else if bytes.HasPrefix(rawMetadataField, []byte("h:")) {
			hostFromField = string(rawMetadataField[2:])
		} else if bytes.HasPrefix(rawMetadataField, []byte("#")) {
			service.Tags, hostFromTags = parseTags(rawMetadataField[1:], defaultHostname)
		} else if bytes.HasPrefix(rawMetadataField, []byte("m:")) {
			service.Message = string(rawMetadataField[2:])
		} else {
			log.Warnf("unknown metadata type: '%s'", rawMetadataField)
		}
	}

	if hostFromField != "" {
		service.Host = hostFromField
	} else {
		service.Host = hostFromTags
	}
	return &service, nil
}

func parseEventMessage(message []byte, defaultHostname string) (*metrics.Event, error) {
	// _e{title.length,text.length}:title|text
	//  [
	//   |d:date_happened
	//   |p:priority
	//   |h:hostname
	//   |t:alert_type
	//   |s:source_type_nam
	//   |#tag1,tag2
	//  ]

	messageRaw := bytes.SplitN(message, []byte(":"), 2)
	if len(messageRaw) < 2 || len(messageRaw[0]) < 7 || len(messageRaw[1]) < 3 {
		return nil, fmt.Errorf("Invalid message format")
	}
	header := messageRaw[0]
	message = messageRaw[1]

	rawLen := bytes.SplitN(header[3:], []byte(","), 2)
	if len(rawLen) != 2 {
		return nil, fmt.Errorf("Invalid message format")
	}

	titleLen, err := parseInt64(rawLen[0])
	if err != nil {
		return nil, fmt.Errorf("Invalid message format, could not parse title.length: '%s'", rawLen[0])
	}

	textLen, err := parseInt64(rawLen[1][:len(rawLen[1])-1])
	if err != nil {
		return nil, fmt.Errorf("Invalid message format, could not parse text.length: '%s'", rawLen[0])
	}
	if titleLen+textLen+1 > int64(len(message)) {
		return nil, fmt.Errorf("Invalid message format, title.length and text.length exceed total message length")
	}

	rawTitle := message[:titleLen]
	rawText := message[titleLen+1 : titleLen+1+textLen]
	message = message[titleLen+1+textLen:]

	if len(rawTitle) == 0 || len(rawText) == 0 {
		return nil, fmt.Errorf("Invalid event message format: empty 'title' or 'text' field")
	}

	event := metrics.Event{
		Priority:  metrics.EventPriorityNormal,
		AlertType: metrics.EventAlertTypeInfo,
		Title:     string(rawTitle),
		Text:      string(bytes.Replace(rawText, []byte("\\n"), []byte("\n"), -1)),
	}

	// Handle hostname, with a priority to the h: field, then the host:
	// tag and finally the defaultHostname value
	var hostFromField string
	hostFromTags := defaultHostname

	// Metadata
	if len(message) > 1 {
		rawMetadataFields := bytes.Split(message[1:], []byte("|"))

		for i := range rawMetadataFields {
			if bytes.HasPrefix(rawMetadataFields[i], []byte("d:")) {
				ts, err := parseInt64(rawMetadataFields[i][2:])
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
				hostFromField = string(rawMetadataFields[i][2:])
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
				event.Tags, hostFromTags = parseTags(rawMetadataFields[i][1:], defaultHostname)
			} else {
				log.Warnf("unknown metadata type: '%s'", rawMetadataFields[i])
			}
		}
	}

	if hostFromField != "" {
		event.Host = hostFromField
	} else {
		event.Host = hostFromTags
	}
	return &event, nil
}

func parseMetricMessage(message []byte, namespace string, namespaceBlacklist []string, defaultHostname string) (*metrics.MetricSample, error) {
	// daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2
	// daemon:666|g|@0.1|#sometag:somevalue"

	separatorCount := bytes.Count(message, fieldSeparator)
	if separatorCount < 1 || separatorCount > 3 {
		return nil, fmt.Errorf("invalid field number for %q", message)
	}

	// Extract name, value and type
	rawNameAndValue, remainder := nextField(message, fieldSeparator)
	rawName, rawValue := nextField(rawNameAndValue, valueSeparator)
	if rawValue == nil {
		return nil, fmt.Errorf("invalid field format for %q", message)
	}

	rawType, remainder := nextField(remainder, fieldSeparator)
	if len(rawName) == 0 || len(rawValue) == 0 || len(rawType) == 0 {
		return nil, fmt.Errorf("invalid metric message format: empty 'name', 'value' or 'text' field")
	}

	// Metadata
	var metricTags []string
	host := defaultHostname
	var rawMetadataField []byte
	sampleRate := 1.0

	for {
		rawMetadataField, remainder = nextField(remainder, fieldSeparator)

		if bytes.HasPrefix(rawMetadataField, []byte("#")) {
			metricTags, host = parseTags(rawMetadataField[1:], defaultHostname)
		} else if bytes.HasPrefix(rawMetadataField, []byte("@")) {
			rawSampleRate := rawMetadataField[1:]
			var err error
			sampleRate, err = parseFloat64(rawSampleRate)
			if err != nil {
				return nil, fmt.Errorf("invalid sample value for %q", message)
			}
		}

		if remainder == nil {
			break
		}
	}

	metricName := string(rawName)
	if namespace != "" {
		blacklisted := false
		for _, prefix := range namespaceBlacklist {
			if strings.HasPrefix(metricName, prefix) {
				blacklisted = true
			}
		}
		if !blacklisted {
			metricName = namespace + metricName
		}
	}

	metricType, ok := metricTypes[string(rawType)]
	if !ok {
		return nil, fmt.Errorf("invalid metric type for %q", message)
	}

	sample := &metrics.MetricSample{
		Name:       metricName,
		Mtype:      metricType,
		Tags:       metricTags,
		Host:       host,
		SampleRate: sampleRate,
		Timestamp:  0,
	}

	if metricType == metrics.SetType {
		sample.RawValue = string(rawValue)
	} else {
		metricValue, err := parseFloat64(rawValue)
		if err != nil {
			return nil, fmt.Errorf("invalid metric value for %q", message)
		}
		sample.Value = metricValue
	}

	return sample, nil
}

// the std API does not have methods to do []byte => float parsing
// we use this unsafe trick to avoid having to allocate one string for
// every parsed float
// see https://github.com/golang/go/issues/2632
func parseFloat64(rawFloat []byte) (float64, error) {
	return strconv.ParseFloat(*(*string)(unsafe.Pointer(&rawFloat)), 64)
}

// the std API does not have methods to do []byte => float parsing
// we use this unsafe trick to avoid having to allocate one string for
// every parsed float
// see https://github.com/golang/go/issues/2632
func parseInt64(rawInt []byte) (int64, error) {
	return strconv.ParseInt(*(*string)(unsafe.Pointer(&rawInt)), 10, 64)
}
