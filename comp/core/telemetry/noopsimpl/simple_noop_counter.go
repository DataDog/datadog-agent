// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package noopsimpl

type simpleNoOpCounter struct{}

// Inc increments the counter.
func (s *simpleNoOpCounter) Inc() {}

// Add increments the counter by given amount.
func (s *simpleNoOpCounter) Add(float64) {}

// Get gets the current counter value
func (s *simpleNoOpCounter) Get() float64 {
	return 0
}
