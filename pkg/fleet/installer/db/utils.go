// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package db

import "time"

type options struct {
	timeout time.Duration
}

// Option is a function that sets an option on a PackagesDB
type Option func(*options)

// WithTimeout sets the timeout for opening the database
func WithTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.timeout = timeout
	}
}
