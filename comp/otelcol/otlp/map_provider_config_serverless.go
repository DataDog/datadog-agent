// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && serverless
// +build otlp,serverless

package otlp

// defaultTracesConfig is the base traces OTLP pipeline configuration.
// This pipeline is extended through the datadog.yaml configuration values.
// It is written in YAML because it is easier to read and write than a map.
const defaultTracesConfig string = `
receivers:
  otlp:

exporters:
  otlp:
    tls:
      insecure: true
    compression: none

service:
  telemetry:
    metrics:
      level: none
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlp]
`

// defaultMetricsConfig is the metrics OTLP pipeline configuration.
const defaultMetricsConfig string = `
receivers:
  otlp:

exporters:
  serializer:

service:
  telemetry:
    metrics:
      level: none
  pipelines:
    metrics:
      receivers: [otlp]
      exporters: [serializer]
`

// defaultLogsConfig is the logs OTLP pipeline configuration.
const defaultLogsConfig string = ``
