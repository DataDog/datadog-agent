// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import "github.com/DataDog/datadog-agent/pkg/metrics/model"

// Counter tracks how many times something happened per second. Counters are
// only used by DogStatsD and are very similar to Count: the main diffence is
// that they are sent as Rate.
type Counter = model.Counter
