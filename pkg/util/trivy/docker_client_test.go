// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && docker

package trivy

import (
	"context"
	"errors"

	dimage "github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
)

// errSave is the error of imageClient's ImageSave.
var errSave = errors.New("image save")

// imageClient is a client.ImageAPIClient that answers the calls of a scan:
// ImageInspect returns inspect, ImageHistory an empty history, and ImageSave
// fails.
type imageClient struct {
	client.ImageAPIClient
	inspect dimage.InspectResponse
}

func (c imageClient) ImageInspect(context.Context, string, ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	return client.ImageInspectResult{InspectResponse: c.inspect}, nil
}

func (c imageClient) ImageHistory(context.Context, string, ...client.ImageHistoryOption) (client.ImageHistoryResult, error) {
	return client.ImageHistoryResult{}, nil
}

func (c imageClient) ImageSave(context.Context, []string, ...client.ImageSaveOption) (client.ImageSaveResult, error) {
	return nil, errSave
}
