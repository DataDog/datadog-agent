// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"context"
	"errors"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrDockerKindNotSupported is returned when an unsupported kind of docker
// object is requested by check
var ErrDockerKindNotSupported = errors.New("unsupported docker object kind")

// Client abstracts Docker API client
type Client client.CommonAPIClient

type dockerCheck struct {
	baseCheck

	client         Client
	dockerResource *compliance.DockerResource
}

type iterFn func(id string, obj interface{})

func (c *dockerCheck) iterate(ctx context.Context, fn iterFn) error {
	switch c.dockerResource.Kind {
	case "image":
		images, err := c.client.ImageList(ctx, types.ImageListOptions{})
		if err != nil {
			return err
		}
		for _, i := range images {
			imageInspect, _, err := c.client.ImageInspectWithRaw(ctx, i.ID)
			if err != nil {
				// TODO: log here
			}
			fn(i.ID, imageInspect)
		}
	}
	return ErrDockerKindNotSupported
}

func (c *dockerCheck) Run() error {
	// TODO: timeout for checks here
	ctx := context.Background()
	return c.iterate(ctx, func(id string, obj interface{}) {
		log.Debugf("Iterating %s[id=%s]", c.dockerResource.Kind, id)
	})
}
