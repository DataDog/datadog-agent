// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//
// Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may not
// use this file except in compliance with the License. A copy of the
// License is located at
//
// http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
// either express or implied. See the License for the specific language governing
// permissions and limitations under the License.

// The code that follows is mostly based on OpenDataChannel and Execute functions in github.com/aws/session-manager-plugin/src/sessionmanagerplugin/session/sessionhandler.go

//nolint:revive,errcheck
package ecs

import (
	"fmt"
	"math/rand"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/session-manager-plugin/src/config"
	"github.com/aws/session-manager-plugin/src/datachannel"
	"github.com/aws/session-manager-plugin/src/log"
	"github.com/aws/session-manager-plugin/src/message"
	"github.com/aws/session-manager-plugin/src/retry"
	"github.com/aws/session-manager-plugin/src/sessionmanagerplugin/session"
	"github.com/google/uuid"
)

// Slightly modified version of OpenDataChannel function in github.com/aws/session-manager-plugin/src/sessionmanagerplugin/session/sessionhandler.go
func openDataChannel(s *session.Session, logger log.T, stopChan chan bool) (err error) {
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
		func(_ error) {
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
func execute(s *session.Session, logger log.T) (string, error) {
	stopChannel := make(chan bool)
	var payload []byte
	if err := openDataChannel(s, logger, stopChannel); err != nil {
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

// retrieveResultFromExecOutput that allows to retrieve the result from the output of ecs ExecuteCommand method. It uses session-manager-plugin to retrieve the output.
func retrieveResultFromExecOutput(c *Client, output *ecs.ExecuteCommandOutput, task, container string) (string, error) {
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
	return execute(&sess, logger)
}
