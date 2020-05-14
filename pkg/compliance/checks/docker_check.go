// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"context"
	"errors"
	"strings"
	"text/template"

	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrDockerKindNotSupported is returned when an unsupported kind of docker
// object is requested by check
var ErrDockerKindNotSupported = errors.New("unsupported docker object kind")

// DockerClient abstracts Docker API client
type DockerClient interface {
	ImageInspectWithRaw(ctx context.Context, image string) (types.ImageInspect, []byte, error)
	ImageList(ctx context.Context, options types.ImageListOptions) ([]types.ImageSummary, error)
}

type dockerCheck struct {
	baseCheck

	client         DockerClient
	dockerResource *compliance.DockerResource
}

type iterFn func(id string, obj interface{})

func (c *dockerCheck) iterate(ctx context.Context, fn iterFn) error {
	switch c.dockerResource.Kind {
	case "image":
		images, err := c.client.ImageList(ctx, types.ImageListOptions{All: true})
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
		return nil
	}
	return ErrDockerKindNotSupported
}

func (c *dockerCheck) Run() error {
	// TODO: timeout for checks here
	ctx := context.Background()
	return c.iterate(ctx, c.inspect)
}

func (c *dockerCheck) inspect(id string, obj interface{}) {
	log.Debugf("Iterating %s[id=%s]", c.dockerResource.Kind, id)

	for _, f := range c.dockerResource.Filter {
		if f.Include != nil {
			eval, err := evalTemplate(f.Include.Exists, obj)
			if err != nil || eval == "" {
				return
			}
		} else if f.Exclude != nil {
			eval, err := evalTemplate(f.Exclude.Exists, obj)
			if err == nil && eval != "" {
				return
			}
		}
	}

	kv := compliance.KV{}
	for _, field := range c.dockerResource.Report {

		key := field.As

		if field.Value != "" {
			if key == "" {
				// TODO: erorr here
			}

			kv[key] = field.Value
			continue
		}

		if field.Attribute == "id" {
			if key == "" {
				key = "id"
			}
			kv[key] = id
			continue
		}
	}

	c.report(nil, kv)
}

func evalTemplate(s string, obj interface{}) (string, error) {
	tmpl, err := template.New("tmpl").Parse(s)
	if err != nil {
		return "", err
	}

	b := &strings.Builder{}
	if err := tmpl.Execute(b, obj); err != nil {
		return "", err
	}
	return b.String(), nil
}
