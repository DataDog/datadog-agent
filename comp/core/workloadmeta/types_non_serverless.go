// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package workloadmeta

import (
	"fmt"
	"io"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// ContainerImageLayer represents a layer of a container image
type ContainerImageLayer struct {
	MediaType string
	Digest    string
	SizeBytes int64
	URLs      []string
	History   v1.History
}

func printHistory(out io.Writer, history v1History) {
	if history == nil {
		_, _ = fmt.Fprintln(out, "History is nil")
		return
	}

	_, _ = fmt.Fprintln(out, "History:")
	_, _ = fmt.Fprintln(out, "- createdAt:", history.Created())
	_, _ = fmt.Fprintln(out, "- createdBy:", history.CreatedBy())
	_, _ = fmt.Fprintln(out, "- comment:", history.Comment())
	_, _ = fmt.Fprintln(out, "- emptyLayer:", history.EmptyLayer())
}
