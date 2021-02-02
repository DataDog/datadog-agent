// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/docker/docker/api/types"
)

var (
	dockerReportedFields = []string{
		compliance.DockerImageFieldID,
		compliance.DockerImageFieldTags,
		compliance.DockerContainerFieldID,
		compliance.DockerContainerFieldName,
		compliance.DockerContainerFieldImage,
		compliance.DockerNetworkFieldName,
		compliance.DockerVersionFieldVersion,
	}
)

func dockerKindNotSupported(kind string) error {
	return fmt.Errorf("unsupported docker object kind '%s'", kind)
}

func resolveDocker(ctx context.Context, e env.Env, ruleID string, res compliance.Resource) (interface{}, error) {
	if res.Docker == nil {
		return nil, fmt.Errorf("expecting docker resource in docker check")
	}

	client := e.DockerClient()
	if client == nil {
		return nil, fmt.Errorf("docker client not configured")
	}

	switch res.Docker.Kind {
	case "image":
		return newDockerImageIterator(ctx, client)
	case "container":
		return newDockerContainerIterator(ctx, client)
	case "network":
		return newDockerNetworkIterator(ctx, client)
	case "info":
		return newDockerInfoInstance(ctx, client)
	case "version":
		return newDockerVersionInstance(ctx, client)
	default:
		return nil, dockerKindNotSupported(res.Docker.Kind)
	}
}

func newDockerInfoInstance(ctx context.Context, client env.DockerClient) (*eval.Instance, error) {
	info, err := client.Info(ctx)
	if err != nil {
		return nil, err
	}

	return &eval.Instance{
		Functions: eval.FunctionMap{
			compliance.DockerFuncTemplate: dockerTemplateQuery(compliance.DockerFuncTemplate, info),
		},
	}, nil
}

func newDockerVersionInstance(ctx context.Context, client env.DockerClient) (*eval.Instance, error) {
	version, err := client.ServerVersion(ctx)
	if err != nil {
		return nil, err
	}

	return &eval.Instance{
		Vars: eval.VarMap{
			compliance.DockerVersionFieldVersion:       version.Version,
			compliance.DockerVersionFieldAPIVersion:    version.APIVersion,
			compliance.DockerVersionFieldPlatform:      version.Platform.Name,
			compliance.DockerVersionFieldExperimental:  version.Experimental,
			compliance.DockerVersionFieldOS:            version.Os,
			compliance.DockerVersionFieldArch:          version.Arch,
			compliance.DokcerVersionFieldKernelVersion: version.KernelVersion,
		},
		Functions: eval.FunctionMap{
			compliance.DockerFuncTemplate: dockerTemplateQuery(compliance.DockerFuncTemplate, version),
		},
	}, nil
}

func dockerTemplateQuery(funcName, obj interface{}) eval.Function {
	return func(_ *eval.Instance, args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf(`invalid number of arguments in "%s()", expecting 1 got %d`, funcName, len(args))
		}

		query, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf(`expecting string value for query argument in "%s()"`, funcName)
		}

		v := evalGoTemplate(query, obj)
		log.Tracef(`template query in "%s(%q)" evaluated as %q`, funcName, query, v)
		return v, nil
	}
}

type dockerImageIterator struct {
	ctx    context.Context
	client env.DockerClient
	images []types.ImageSummary
	index  int
}

func newDockerImageIterator(ctx context.Context, client env.DockerClient) (eval.Iterator, error) {
	images, err := client.ImageList(ctx, types.ImageListOptions{All: true})
	if err != nil {
		return nil, err
	}

	return &dockerImageIterator{
		ctx:    ctx,
		client: client,
		images: images,
	}, nil
}

func (it *dockerImageIterator) Next() (*eval.Instance, error) {
	if it.Done() {
		return nil, ErrInvalidIteration
	}

	image := it.images[it.index]

	imageInspect, _, err := it.client.ImageInspectWithRaw(it.ctx, image.ID)
	if err != nil {
		return nil, log.Errorf("failed to inspect image %s", image.ID)
	}

	it.index++

	return &eval.Instance{
		Vars: eval.VarMap{
			compliance.DockerImageFieldID:   image.ID,
			compliance.DockerImageFieldTags: imageInspect.RepoTags,
		},
		Functions: eval.FunctionMap{
			compliance.DockerFuncTemplate: dockerTemplateQuery(compliance.DockerFuncTemplate, imageInspect),
		},
	}, nil
}

func (it *dockerImageIterator) Done() bool {
	return it.index >= len(it.images)
}

type dockerContainerIterator struct {
	ctx        context.Context
	client     env.DockerClient
	containers []types.Container
	index      int
}

func newDockerContainerIterator(ctx context.Context, client env.DockerClient) (eval.Iterator, error) {
	containers, err := client.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}

	return &dockerContainerIterator{
		ctx:        ctx,
		client:     client,
		containers: containers,
	}, nil
}

func (it *dockerContainerIterator) Next() (*eval.Instance, error) {
	if it.Done() {
		return nil, ErrInvalidIteration
	}

	container := it.containers[it.index]

	containerInspect, err := it.client.ContainerInspect(it.ctx, container.ID)
	if err != nil {
		return nil, log.Errorf("failed to inspect container %s", container.ID)
	}

	it.index++

	return &eval.Instance{
		Vars: eval.VarMap{
			compliance.DockerContainerFieldID:    container.ID,
			compliance.DockerContainerFieldName:  containerInspect.Name,
			compliance.DockerContainerFieldImage: containerInspect.Image,
		},
		Functions: eval.FunctionMap{
			compliance.DockerFuncTemplate: dockerTemplateQuery(compliance.DockerFuncTemplate, containerInspect),
		},
	}, nil
}

func (it *dockerContainerIterator) Done() bool {
	return it.index >= len(it.containers)
}

type dockerNetworkIterator struct {
	ctx      context.Context
	client   env.DockerClient
	networks []types.NetworkResource
	index    int
}

func newDockerNetworkIterator(ctx context.Context, client env.DockerClient) (eval.Iterator, error) {
	networks, err := client.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, err
	}

	return &dockerNetworkIterator{
		ctx:      ctx,
		client:   client,
		networks: networks,
	}, nil
}

func (it *dockerNetworkIterator) Next() (*eval.Instance, error) {
	if it.Done() {
		return nil, ErrInvalidIteration
	}

	network := it.networks[it.index]

	it.index++

	return &eval.Instance{
		Vars: eval.VarMap{
			compliance.DockerNetworkFieldID:   network.ID,
			compliance.DockerNetworkFieldName: network.Name,
		},
		Functions: eval.FunctionMap{
			compliance.DockerFuncTemplate: dockerTemplateQuery(compliance.DockerFuncTemplate, network),
		},
	}, nil
}

func (it *dockerNetworkIterator) Done() bool {
	return it.index >= len(it.networks)
}
