// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func BenchmarkParsePacket(b *testing.B) {
	s, _ := NewServer(nil, nil, nil)
	defer s.Stop()

	for n := 0; n < b.N; n++ {
		packet := listeners.Packet{
			Contents: []byte("daemon:666|h|@0.5|#sometag1:somevalue1,sometag2:somevalue2"),
			Origin:   listeners.NoOrigin,
		}
		s.parsePacket(&packet, []*metrics.MetricSample{}, []*metrics.Event{}, []*metrics.ServiceCheck{})
	}
}

// Benchmark result without cache:
// 	BenchmarkMapper-8   	  500000	      2010 ns/op
//
// Benchmark with cache:
//  BenchmarkMapper-8   	 2000000	       885 ns/op

func BenchmarkMapper(b *testing.B) {
	datadogYaml := `
dogstatsd_mappings:
  - match: "airflow.job.duration.*.*"       # metric format: airflow.job.duration.<job_type>.<job_name>
    name: "airflow.job.duration"            # remap the metric name
    tags:
      job_type: "$1"
      job_name: "$2"
  - match: "airflow.job.size.*.*"           # metric format: airflow.job.size.<job_type>.<job_name>
    name: "airflow.job.size"                # remap the metric name
    tags:
      foo: "$1"
      bar: "$2"
`
	config.Datadog.SetConfigType("yaml")
	config.Datadog.ReadConfig(strings.NewReader(datadogYaml))

	s, _ := NewServer(nil, nil, nil)
	defer s.Stop()

	for n := 0; n < b.N; n++ {
		packet := listeners.Packet{
			Contents: []byte("airflow.job.duration.my_job_type.my_job_name:666|g"),
			Origin:   listeners.NoOrigin,
		}
		s.parsePacket(&packet, []*metrics.MetricSample{}, []*metrics.Event{}, []*metrics.ServiceCheck{})
	}
}
