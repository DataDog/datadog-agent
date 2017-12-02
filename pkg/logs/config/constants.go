// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package config

// Technical constants

const (
	ChanSizes         = 100
	NumberOfPipelines = int32(4)
)

// Business constants

const (
	DateFormat = "2006-01-02T15:04:05.000000000Z"
)

var (
	SEV_INFO  = []byte("<46>")
	SEV_ERROR = []byte("<43>")
)
