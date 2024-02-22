// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package workloadmeta

import (
	"io"
)

// ContainerImageLayer represents a layer of a container image
type ContainerImageLayer struct {
	MediaType string
	Digest    string
	SizeBytes int64
	URLs      []string
	History   noOpV1History
}

type noOpV1History struct{}

func (noOpV1History) Created() string {
	return ""
}

func (noOpV1History) CreatedBy() string {
	return ""
}

func (noOpV1History) Comment() string {
	return ""
}

func (noOpV1History) EmptyLayer() string {
	return ""
}

func printHistory(out io.Writer, history v1History) {
	// nothing to do here
}
