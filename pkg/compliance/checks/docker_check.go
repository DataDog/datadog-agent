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
	"time"

	"github.com/Masterminds/sprig"
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
	ServerVersion(ctx context.Context) (types.Version, error)
	Close() error
}

type dockerCheck struct {
	baseCheck

	client         DockerClient
	dockerResource *compliance.DockerResource
}

func newDockerCheck(baseCheck baseCheck, client DockerClient, dockerResource *compliance.DockerResource) (*dockerCheck, error) {
	// TODO: validate config for the docker resource here
	return &dockerCheck{
		baseCheck:      baseCheck,
		client:         client,
		dockerResource: dockerResource,
	}, nil
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
	case "info":
		info, err := c.client.Info(ctx)
		if err != nil {
			return err
		}
		fn("", info)
	case "version":
		version, err := c.client.ServerVersion(ctx)
		if err != nil {
			return err
		}
		fn("", version)
	default:
		return ErrDockerKindNotSupported
	}
	return nil
}

func (c *dockerCheck) Run() error {
	log.Debugf("%s: running docker check", c.id)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(defaultTimeoutSeconds)*time.Second)
	defer cancel()
	return c.iterate(ctx, c.inspect)
}

func (c *dockerCheck) inspect(id string, obj interface{}) {
	log.Debugf("%s: iterating %s[id=%s]", c.id, c.dockerResource.Kind, id)

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

	kv := compliance.KVMap{}
	for _, field := range c.dockerResource.Report {
		if c.setStaticKV(field, kv) {
			continue
		}

		key := field.As
		if field.Kind == compliance.PropertyKindTemplate {
			if key == "" {
				log.Errorf("%s: template field without an alias key - %s", c.id, field.Property)
				continue
			}
			kv[key] = evalTemplate(field.Property, obj)
		}

		if field.Property == "id" {
			if key == "" {
				key = "id"
			}
			kv[key] = id
			continue
		}
	}

	c.report(nil, kv, "%s[id=%s]", c.dockerResource.Kind, id)
}

func evalCondition(property string, condition *compliance.Condition) bool {
	switch condition.Operation {
	case compliance.OpExists, "":
		return property != ""

	case compliance.OpEqual:
		return property == condition.Value
	default:
		log.Warnf("Unsupported operation in condition: %s", condition.Operation)
		return false
	}
}

func evalTemplate(s string, obj interface{}) string {
	tmpl, err := template.New("tmpl").Funcs(sprig.TxtFuncMap()).Parse(s)
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
