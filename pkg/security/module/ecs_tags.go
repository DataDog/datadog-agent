// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package module holds module related files
package module

import (
	"context"
	"strings"
	"time"

	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
)

func getCurrentECSTaskTags() (map[string]string, error) {
	client, err := ecsmeta.V3orV4FromCurrentTask()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5*time.Second))
	defer cancel()

	task, err := client.GetTask(ctx)
	if err != nil {
		return nil, err
	}

	cont, err := client.GetContainer(ctx)
	if err != nil {
		return nil, err
	}
	imageName := cont.Name
	imageTag := ""
	image := strings.Split(cont.Image, ":")
	if len(image) == 2 {
		imageName = image[0]
		imageTag = image[1]
	}

	return map[string]string{
		"task_name":    task.Family,
		"task_family":  task.Family,
		"task_arn":     task.TaskARN,
		"task_version": task.Version,
		"image_name":   imageName,
		"image_tag":    imageTag,
	}, nil
}
