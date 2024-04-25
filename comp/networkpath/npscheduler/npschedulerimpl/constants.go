// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npschedulerimpl

import "time"

const (
	// DefaultFlushTickerInterval is the default flush interval in seconds
	DefaultFlushTickerInterval = 10 * time.Second

	// DefaultPathtestRunDurationFromDiscovery is the default path test run duration
	DefaultPathtestRunDurationFromDiscovery = 10 * time.Minute

	// DefaultPathtestRunInterval is the default path test run duration
	DefaultPathtestRunInterval = 1 * time.Minute
)
