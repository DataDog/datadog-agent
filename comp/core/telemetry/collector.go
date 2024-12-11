// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !serverless

package telemetry

import "github.com/prometheus/client_golang/prometheus"

// Collector is an alias to prometheus.Collector
type Collector = prometheus.Collector
