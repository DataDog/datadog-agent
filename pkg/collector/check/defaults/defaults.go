// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaults

import (
	"time"
)

const (
	// DefaultCheckInterval is the interval in seconds the scheduler should apply
	// when no value was provided in Check configuration.
	DefaultCheckInterval time.Duration = 15 * time.Second
)
