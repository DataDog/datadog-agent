// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ecs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/cenkalti/backoff/v4"
)

// Client is a client for ECS
type Client struct {
	ecs.Client
	clusterName string
}

// NewClient creates a new ECS client
func NewClient(clusterName string) (*Client, error) {
	ctx := context.Background()
	cfg, err := awsConfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	return &Client{Client: *ecs.NewFromConfig(cfg), clusterName: clusterName}, err
}

// ExecCommand executes a command in a container in a task in an ECS cluster.
// It accepts either the task ARN or the task ID.
// WARNING: This function will return a nil error as soon as it succeed to execute the command and retrieve the output, even if the command executed failed
// WARNING: This function will not work on Fargate tasks with pidMode=task per https://github.com/aws/containers-roadmap/issues/2268
func (c *Client) ExecCommand(task, containerName string, cmd string) (string, error) {
	taskID := task
	if strings.HasPrefix(task, "arn:") {
		taskArnSplit := strings.Split(task, "/")
		taskID = taskArnSplit[len(taskArnSplit)-1]
	}

	// Check that ExecCommand Agent is running
	err := backoff.Retry(func() error {
		tasks, err := c.DescribeTasks(context.Background(), &ecs.DescribeTasksInput{
			Cluster: aws.String(c.clusterName),
			Tasks:   []string{task},
		})
		if err != nil {
			return err
		}

		for _, container := range tasks.Tasks[0].Containers {
			if *container.Name == containerName {
				if *container.ManagedAgents[0].LastStatus != "RUNNING" {
					return errors.New("agent not running")
				}
			}
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), 5))
	if err != nil {
		return "", err
	}

	output, err := c.ExecuteCommand(context.Background(), &ecs.ExecuteCommandInput{
		Cluster:     aws.String(c.clusterName),
		Container:   aws.String(containerName),
		Task:        aws.String(taskID),
		Command:     aws.String(cmd),
		Interactive: true,
	})
	if err != nil {
		return "", err
	}
	return retrieveResultFromExecOutput(c, output, taskID, containerName)
}

func (c *Client) getContainerRuntime(task, containerName string) (string, error) {
	res, err := c.DescribeTasks(context.Background(), &ecs.DescribeTasksInput{
		Cluster: aws.String(c.clusterName),
		Tasks:   []string{task},
	})
	if err != nil {
		return "", err
	}
	for _, task := range res.Tasks {
		for _, container := range task.Containers {
			if *container.Name == containerName {
				return *container.RuntimeId, nil
			}
		}
	}
	return "", fmt.Errorf("container %s not found in task %s", containerName, task)
}
