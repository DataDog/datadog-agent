// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package util

// OptionalFloat64 is an optional float64
type OptionalFloat64 struct {
	value float64
	isSet bool
}

// NewOptionalFloat64 creates a new instance of OptionalFloat64
func NewOptionalFloat64() *OptionalFloat64 {
	return &OptionalFloat64{
		value: 0,
		isSet: false,
	}
}

// Set sets the value
func (o *OptionalFloat64) Set(v float64) {
	o.value = v
	o.isSet = true
}

// Get returns if a value is set and the value
func (o *OptionalFloat64) Get() (float64, bool) {
	return o.value, o.isSet
}

// UnSet remove the value previously set
func (o *OptionalFloat64) UnSet() {
	o.isSet = false
}
