// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package noopsimpl

type simpleNoOpGauge struct{}

// Inc increments the gauge.
func (s *simpleNoOpGauge) Inc() {}

// Dec decrements the gauge.
func (s *simpleNoOpGauge) Dec() {}

// Add increments the gauge by given amount.
func (s *simpleNoOpGauge) Add(float64) {}

// Sub decrements the gauge by given amount.
func (s *simpleNoOpGauge) Sub(float64) {}

// Set sets the value of the gauge.
func (s *simpleNoOpGauge) Set(float64) {}

// Get gets the value of the gauge.
func (s *simpleNoOpGauge) Get() float64 {
	return 0
}
