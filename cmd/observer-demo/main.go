// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"flag"

	observerimpl "github.com/DataDog/datadog-agent/comp/observer/impl"
)

func main() {
	timeScale := flag.Float64("timescale", 0.25, "time multiplier (0.25 = 4x faster, runs in 10s)")
	flag.Parse()
	observerimpl.RunDemo(*timeScale)
}
