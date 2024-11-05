// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go/service/ssm"
)

type ECSClient struct {
	ecs.Client
	clusterName string
}

func NewECSClient(clusterName string) (*ECSClient, error) {
	ctx := context.Background()
	cfg, err := awsConfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	return &ECSClient{Client: *ecs.NewFromConfig(cfg), clusterName: clusterName}, err
}

func (c *ECSClient) ExecCommand(task, container string, cmd string) (string, string, error) {
	// Check if session-manager-plugin is installed, fails early if not
	_, err := exec.LookPath("session-manager-plugin")
	if err != nil {
		return "", "", fmt.Errorf("session-manager-plugin not found in PATH, follow https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html to install it")
	}

	output, err := c.ExecuteCommand(context.Background(), &ecs.ExecuteCommandInput{
		Cluster:     aws.String(c.clusterName),
		Container:   aws.String(container),
		Task:        aws.String(task),
		Command:     aws.String(cmd),
		Interactive: true,
	})
	if err != nil {
		return "", "", err
	}
	containerRuntime, err := c.getContainerRuntime(task, container)
	if err != nil {
		return "", "", err
	}

	targetSSM := ssm.StartSessionInput{
		Target: aws.String(fmt.Sprintf("ecs:%s_%s_%s", c.clusterName, task, containerRuntime)),
	}
	targetJson, err := json.Marshal(targetSSM)
	if err != nil {
		return "", "", err
	}

	execSess, err := json.Marshal(output.Session)
	if err != nil {
		return "", "", err
	}

	ssmCmd := exec.Command("session-manager-plugin", string(execSess), "us-east-1", "StartSession", "", string(targetJson))
	if err != nil {
		return "", "", err
	}
	var stdOutBuffer bytes.Buffer
	var stdErrBuffer bytes.Buffer

	ssmCmd.Stdout = &stdOutBuffer
	ssmCmd.Stderr = &stdErrBuffer
	ssmCmd.Stdin = os.Stdin // It fails if omitted
	err = ssmCmd.Run()
	if err != nil {
		return "", "", err
	}

	return string(stdOutBuffer.Bytes()), string(stdErrBuffer.Bytes()), nil
}

func (c *ECSClient) getContainerRuntime(task, containerName string) (string, error) {
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
