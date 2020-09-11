// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package remote

// OngoingFlareError is used to warn about ongoing flares
type OngoingFlareError struct {
	err string
}

// InvalidLogType is used to warn about invalid log types
type InvalidLogType struct {
	err string
}

// InvalidTracerId is used to warn about invalid tracer id in requests
type InvalidTracerId struct {
	err string
}

// InvalidFlareId is used to warn about invalid flare id in requests
type InvalidFlareId struct {
	err string
}

func (e OngoingFlareError) Error() string {
	return e.err
}

func (e InvalidLogType) Error() string {
	return e.err
}

func (e InvalidTracerId) Error() string {
	return e.err
}

func (e InvalidFlareId) Error() string {
	return e.err
}
