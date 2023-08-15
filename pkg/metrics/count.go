// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import "github.com/DataDog/datadog-agent/pkg/metrics/model"

// Count is used to count the number of events that occur between 2 flushes. Each sample's value is added
// to the value that's flushed
type Count = model.Count
