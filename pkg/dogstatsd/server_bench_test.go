// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
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

var airflowMappingYaml = `---
mappings:
#- match: airflow\.(\w+)_start
#  name: "airflow_job_start"
#  match_type: regex
#  labels:
#    job_name: "$1"
#- match: airflow\.(\w+)_end
#  name: "airflow_job_end"
#  match_type: regex
#  labels:
#    job_name: "$1"
#- match: airflow\.operator_failures_(\w+)
#  name: "airflow_operator_failures"
#  match_type: regex
#  labels:
#    operator_name: "$1"
#- match: airflow\.operator_successes_(\w+)
#  name: "airflow_operator_successes"
#  match_type: regex
#  labels:
#    operator_name: "$1"
- match: airflow.dag_processing.last_runtime.*
  name: "airflow_dag_processing_last_runtime"
  labels:
    dag_file: "$1"
- match: airflow.dag_processing.last_run.seconds_ago.*
  name: "airflow_dag_processing_last_run_seconds_ago"
  labels:
    dag_file: "$1"
- match: airflow.pool.open_slots.*
  name: "airflow_pool_open_slots"
  labels:
    pool_name: "$1"
- match: airflow.pool.used_slots.*
  name: "airflow_pool_used_slots"
  labels:
    pool_name: "$1"
- match: airflow.pool.starving_tasks.*
  name: "airflow_pool_starving_tasks"
  labels:
    pool_name: "$1"
- match: airflow.dagrun.dependency-check.*
  name: "airflow_dagrun_dependency_check"
  labels:
    dag_id: "$1"
- match: airflow.dag.*.*.duration
  name: "airflow_dag_duration"
  labels:
    dag_id: "$1"
    task_id: "$2"
- match: airflow.dag_processing.last_duration.*
  name: "airflow_dag_processing_last_duration"
  labels:
    dag_file: "$1"
- match: airflow.dagrun.duration.success.*
  name: "airflow_dagrun_duration_success"
  labels:
    dag_id: "$1"
- match: airflow.dagrun.duration.failed.*
  name: "airflow_dagrun_duration_failed"
  labels:
    dag_id: "$1"
- match: airflow.dagrun.schedule_delay.*
  name: "airflow_dagrun_schedule_delay"
  labels:
    dag_id: "$1"
`

var airflowMetrics = []string{
"airflow.ti_failures:666|g|#sometag1:somevalue1",
}

func BenchmarkMapperAirflowWithoutMapping(b *testing.B) {
	config.Datadog.SetDefault("mapping_yaml", nil)

	s, _ := NewServer(nil, nil, nil)
	defer s.Stop()

	for n := 0; n < b.N; n++ {

		for i := 0; i < 1000; i++ {
			for _, m := range airflowMetrics {
				parseMetric(m, s)
			}
		}

	}
}


func BenchmarkMapperAirflowWithMapping(b *testing.B) {
	config.Datadog.SetDefault("mapping_yaml", airflowMappingYaml)

	s, _ := NewServer(nil, nil, nil)
	defer s.Stop()

	for n := 0; n < b.N; n++ {

		for i := 0; i < 1000; i++ {
			for _, m := range airflowMetrics {
				parseMetric(m, s)
			}
		}

	}
}


func parseMetric(metric string, s *Server) {
	packet := listeners.Packet{
		Contents: []byte(metric),
		Origin:   listeners.NoOrigin,
	}
	s.parsePacket(&packet, []*metrics.MetricSample{}, []*metrics.Event{}, []*metrics.ServiceCheck{})
}
