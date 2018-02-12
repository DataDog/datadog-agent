// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

// Pipeline constraints
const (
	ChanSize          = 100
	NumberOfPipelines = 4
)

const (
	// DateFormat is the default date format.
	DateFormat = "2006-01-02T15:04:05.000000000Z"
	// StatusInfo is the default status for info messages.
	StatusInfo = "info"
	// StatusError is the default status for error messages.
	StatusError = "error"
)

var (
	// SevInfo is the syslog severity for info messages.
	SevInfo = []byte("<46>")
	// SevError is the syslog severity for error messages.
	SevError = []byte("<43>")
)
