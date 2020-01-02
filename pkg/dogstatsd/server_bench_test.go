// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func BenchmarkParsePacket(b *testing.B) {
	port, err := getAvailableUDPPort()
	require.NoError(b, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	s, _ := NewServer(nil, nil, nil)
	defer s.Stop()

	for n := 0; n < b.N; n++ {
		packet := listeners.Packet{
			Contents: []byte("daemon:666|h|@0.5|#sometag1:somevalue1,sometag2:somevalue2"),
			Origin:   listeners.NoOrigin,
		}
		s.parsePacket(&packet, []*metrics.MetricSample{}, []*metrics.Event{}, []*metrics.ServiceCheck{})
	}

	b.ReportAllocs()
}

func BenchmarkWithMapper(b *testing.B) {
	datadogYaml := `
dogstatsd_mapper_profiles:
  - name: airflow
    prefix: 'airflow.'
    mappings:
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
	err := config.Datadog.ReadConfig(strings.NewReader(datadogYaml))
	assert.NoError(b, err)

	BenchmarkMapperControl(b)
}

func BenchmarkMapperControl(b *testing.B) {
	port, err := getAvailableUDPPort()
	require.NoError(b, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	s, _ := NewServer(nil, nil, nil)
	defer s.Stop()

	for n := 0; n < b.N; n++ {
		packet := listeners.Packet{
			Contents: []byte("airflow.job.duration.my_job_type.my_job_name:666|g"),
			Origin:   listeners.NoOrigin,
		}
		s.parsePacket(&packet, []*metrics.MetricSample{}, []*metrics.Event{}, []*metrics.ServiceCheck{})
	}

	b.ReportAllocs()
}
