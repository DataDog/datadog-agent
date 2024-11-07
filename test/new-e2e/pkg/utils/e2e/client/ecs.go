// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package client

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/session-manager-plugin/src/config"
	"github.com/aws/session-manager-plugin/src/datachannel"
	"github.com/aws/session-manager-plugin/src/log"
	"github.com/aws/session-manager-plugin/src/message"
	"github.com/aws/session-manager-plugin/src/retry"
	"github.com/aws/session-manager-plugin/src/sessionmanagerplugin/session"
	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
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

// ExecCommand executes a command in a container in a task in an ECS cluster.
// It accept either the task ARN or the task ID.
// WARNING: This function will return a nil error as soon as it succeed to execute the command and retrieve the output, even if the command executed failed
func (c *ECSClient) ExecCommand(task, containerName string, cmd string) (string, error) {
	taskId := task
	if strings.HasPrefix(task, "arn:") {
		taskArnSplit := strings.Split(task, "/")
		taskId = taskArnSplit[len(taskArnSplit)-1]
	}
	fmt.Println("TASK:", task)
	fmt.Println("TASK ID:", taskId)

	// Check that ExecCommand Agent is running
	backoff.Retry(func() error {
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

	output, err := c.ExecuteCommand(context.Background(), &ecs.ExecuteCommandInput{
		Cluster:     aws.String(c.clusterName),
		Container:   aws.String(containerName),
		Task:        aws.String(taskId),
		Command:     aws.String(cmd),
		Interactive: true,
	})
	if err != nil {
		return "", err
	}
	return c.RetrieveResultFromExecOutput(output, taskId, containerName)
}

func (c *ECSClient) RetrieveResultFromExecOutput(output *ecs.ExecuteCommandOutput, task, container string) (string, error) {
	containerRuntime, err := c.getContainerRuntime(task, container)
	if err != nil {
		return "", err
	}

	sess := session.Session{}
	sess.StreamUrl = *output.Session.StreamUrl
	sess.TokenValue = *output.Session.TokenValue
	sess.SessionId = *output.Session.SessionId
	sess.Endpoint = "https://ecs.us-east-1.amazonaws.com"
	sess.DataChannel = &datachannel.DataChannel{}
	sess.ClientId = uuid.NewString()
	sess.TargetId = fmt.Sprintf("ecs:%s_%s_%s", c.clusterName, task, containerRuntime)
	logger := log.Logger(true, "ecs-execute")
	return Execute(&sess, logger)
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

// Slightly modified version of OpenDataChannel function in github.com/aws/session-manager-plugin/src/sessionmanagerplugin/session/sessionhandler.go
func OpenDataChannel(s *session.Session, logger log.T, stopChan chan bool) (err error) {
	retryParams := retry.RepeatableExponentialRetryer{
		GeometricRatio:      config.RetryBase,
		InitialDelayInMilli: rand.Intn(config.DataChannelRetryInitialDelayMillis) + config.DataChannelRetryInitialDelayMillis,
		MaxDelayInMilli:     config.DataChannelRetryMaxIntervalMillis,
		MaxAttempts:         config.DataChannelNumMaxRetries,
	}

	s.DataChannel.Initialize(logger, s.ClientId, s.SessionId, s.TargetId, s.IsAwsCliUpgradeNeeded)
	s.DataChannel.SetWebsocket(logger, s.StreamUrl, s.TokenValue)
	s.DataChannel.GetWsChannel().SetOnMessage(
		func(input []byte) {
			s.DataChannel.OutputMessageHandler(logger, func() { stopChan <- true }, s.SessionId, input)
		})
	s.DataChannel.RegisterOutputStreamHandler(s.ProcessFirstMessage, false)

	if err = s.DataChannel.Open(logger); err != nil {
		logger.Errorf("Retrying connection for data channel id: %s failed with error: %s", s.SessionId, err)
		retryParams.CallableFunc = func() (err error) { return s.DataChannel.Reconnect(logger) }
		if err = retryParams.Call(); err != nil {
			logger.Error(err)
		}
	}

	s.DataChannel.GetWsChannel().SetOnError(
		func(err error) {
			logger.Errorf("Trying to reconnect the session: %v with seq num: %d", s.StreamUrl, s.DataChannel.GetStreamDataSequenceNumber())
			retryParams.CallableFunc = func() (err error) { return s.ResumeSessionHandler(logger) }
			if err = retryParams.Call(); err != nil {
				logger.Error(err)
			}
		})

	// Scheduler for resending of data
	s.DataChannel.ResendStreamDataMessageScheduler(logger)

	return nil
}

// Slightly modified version of Execute function in github.com/aws/session-manager-plugin/src/sessionmanagerplugin/session/session.go
func Execute(s *session.Session, logger log.T) (string, error) {
	stopChannel := make(chan bool)
	var payload []byte
	if err := OpenDataChannel(s, logger, stopChannel); err != nil {
		logger.Errorf("Error in Opening data channel: %v", err)
		return "", err
	}
	s.DataChannel.RegisterOutputStreamHandler(func(logger log.T, msg message.ClientMessage) (bool, error) {
		payload = append(payload, msg.Payload...)
		return true, nil
	}, true)

	select {
	case <-s.DataChannel.IsSessionTypeSet():
		if <-stopChannel {
			return string(payload), nil
		}
	case <-stopChannel:
		return "", fmt.Errorf("Failed to initialize session")
	}
	return "", nil
}
