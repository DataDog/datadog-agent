// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func BenchmarkParsePacketNoMapping(b *testing.B) {
	s, _ := NewServer(nil, nil, nil)
	defer s.Stop()

	for n := 0; n < b.N; n++ {
		packet := listeners.Packet{
			Contents: []byte("test.dispatcher.a.b.c:666|g"),
			Origin:   listeners.NoOrigin,
		}
		s.parsePacket(&packet, []*metrics.MetricSample{}, []*metrics.Event{}, []*metrics.ServiceCheck{})
	}
}

func BenchmarkMapperMatchingPrefix(b *testing.B) {
	mappingYaml := `---
mappings:
- match: y1.dispatcher
  name: "dispatch_events"
  labels: 
    job: "test_dispatcher"
- match: y2.dispatcher
  name: "dispatch_events"
  labels: 
    job: "test_dispatcher"
- match: y3.dispatcher
  name: "dispatch_events"
  labels: 
    job: "test_dispatcher"
- match: y4.dispatcher
  name: "dispatch_events"
  labels: 
    job: "test_dispatcher"
- match: y5.dispatcher
  name: "dispatch_events"
  labels: 
    job: "test_dispatcher"
- match: y6.dispatcher
  name: "dispatch_events"
  labels: 
    job: "test_dispatcher"
`
	config.Datadog.SetDefault("mapping_yaml", mappingYaml)

	s, _ := NewServer(nil, nil, nil)
	defer s.Stop()

	for n := 0; n < b.N; n++ {
		packet := listeners.Packet{
			Contents: []byte("test.dispatcher.a.b.c:666|g"),
			Origin:   listeners.NoOrigin,
		}
		s.parsePacket(&packet, []*metrics.MetricSample{}, []*metrics.Event{}, []*metrics.ServiceCheck{})
	}
}

func BenchmarkMapperOneMatchingMapping(b *testing.B) {
	mappingYaml := `---
mappings:
- match: test.dispatcher.*.*.*
  name: "dispatch_events"
  labels: 
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
`
	config.Datadog.SetDefault("mapping_yaml", mappingYaml)

	s, _ := NewServer(nil, nil, nil)
	defer s.Stop()

	for n := 0; n < b.N; n++ {
		packet := listeners.Packet{
			Contents: []byte("test.dispatcher.a.b.c:666|g"),
			Origin:   listeners.NoOrigin,
		}
		s.parsePacket(&packet, []*metrics.MetricSample{}, []*metrics.Event{}, []*metrics.ServiceCheck{})
	}
}

func BenchmarkMapperManyMapping(b *testing.B) {
	mappingYaml := `---
mappings:
- match: test.dispatcher.*.*.*
  name: "dispatch_events"
  labels: 
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: test.dispatcher1.*.*.*
  name: "dispatch_events"
  labels: 
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: test.dispatcher2.*.*.*
  name: "dispatch_events"
  labels: 
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: test.dispatcher3.*.*.*
  name: "dispatch_events"
  labels: 
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: test.dispatcher4.*.*.*
  name: "dispatch_events"
  labels: 
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
`
	config.Datadog.SetDefault("mapping_yaml", mappingYaml)

	s, _ := NewServer(nil, nil, nil)
	defer s.Stop()

	for n := 0; n < b.N; n++ {
		packet := listeners.Packet{
			Contents: []byte("test.dispatcher.a.b.c:666|g"),
			Origin:   listeners.NoOrigin,
		}
		s.parsePacket(&packet, []*metrics.MetricSample{}, []*metrics.Event{}, []*metrics.ServiceCheck{})
	}
}
