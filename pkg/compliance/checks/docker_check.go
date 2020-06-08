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
	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrDockerKindNotSupported is returned when an unsupported kind of docker
// object is requested by check
var ErrDockerKindNotSupported = errors.New("unsupported docker object kind")

// DockerClient abstracts Docker API client
type DockerClient interface {
	client.ConfigAPIClient
	client.ContainerAPIClient
	client.ImageAPIClient
	client.NodeAPIClient
	client.NetworkAPIClient
	client.SystemAPIClient
	client.VolumeAPIClient
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
		for _, image := range images {
			imageInspect, _, err := c.client.ImageInspectWithRaw(ctx, image.ID)
			if err != nil {
				log.Errorf("failed to inspect image %s", image.ID)
			}
			fn(image.ID, imageInspect)
		}
	case "container":
		containers, err := c.client.ContainerList(ctx, types.ContainerListOptions{All: true})
		if err != nil {
			return err
		}
		for _, container := range containers {
			containerInspect, err := c.client.ContainerInspect(ctx, container.ID)
			if err != nil {
				log.Errorf("failed to inspect container %s", container.ID)
			}
			fn(container.ID, containerInspect)
		}
	case "network":
		networks, err := c.client.NetworkList(ctx, types.NetworkListOptions{})
		if err != nil {
			return err
		}
		for _, network := range networks {
			fn(network.ID, network)
		}
	default:
		return ErrDockerKindNotSupported
	}
	return nil
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
			prop := evalTemplate(f.Include.Property, obj)
			if !evalCondition(prop, f.Include) {
				return
			}
		} else if f.Exclude != nil {
			prop := evalTemplate(f.Exclude.Property, obj)
			if evalCondition(prop, f.Exclude) {
				return
			}
		}
	}

	kv := compliance.KV{}
	for _, field := range c.dockerResource.Report {

		key := field.As

		if field.Value != "" {
			if key == "" {
				// TODO: error here
				continue
			}

			kv[key] = field.Value
			continue
		}

		if field.Kind == compliance.PropertyKindTemplate {
			if key == "" {
				// TODO: error here
				continue
			}
			value := evalTemplate(field.Property, obj)
			kv[key] = value
		}

		if field.Property == "id" {
			if key == "" {
				key = "id"
			}
			kv[key] = id
			continue
		}
	}

	c.report(nil, kv)
}

func evalCondition(property string, condition *compliance.GenericCondition) bool {
	switch condition.Operation {
	case compliance.OpExists, "":
		return property != ""

	case compliance.OpEqual:
		return property == condition.Value
	default:
		log.Warnf("unsupported operation in condition: %s", condition.Operation)
		return false
	}
}

func evalTemplate(s string, obj interface{}) string {
	tmpl, err := template.New("tmpl").Parse(s)
	if err != nil {
		log.Warn("failed to parse template")
		return ""
	}

	b := &strings.Builder{}
	if err := tmpl.Execute(b, obj); err != nil {
		return ""
	}
	return b.String()
}
