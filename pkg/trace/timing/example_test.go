// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package timing

import (
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// The most basic usage for the package is timing the execution time of a function.
func Example() {
	reporter := New(&statsd.Client{})
	reporter.Start()
	computeThings := func() {
		// the easiest way is using a defer statement, which will trigger
		// when a function returns
		defer reporter.Since("compute.things", time.Now())
		// do stuff...
	}
	computeThings()
	reporter.Stop()
}
