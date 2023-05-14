// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package timing

import "time"

// The most basic usage for the package is timing the execution time of a function.
func Example() {
	computeThings := func() {
		// the easiest way is using a defer statement, which will trigger
		// when a function returns
		defer Since("compute.things", time.Now())
		// do stuff...
	}
	computeThings()
	Stop()
}
