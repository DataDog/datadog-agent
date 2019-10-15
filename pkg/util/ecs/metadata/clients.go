// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

// +build docker

package metadata

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	v3 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3"
)

var globalUtil util

type util struct {
	// used to setup the ECSUtil
	initRetryV1 retry.Retrier
	initRetryV3 retry.Retrier
	initV1      sync.Once
	initV2      sync.Once
	initV3      sync.Once
	v1          *v1.Client
	v2          *v2.Client
	v3          *v3.Client
}

// V1 returns a client for the ECS metadata API v1, also called introspection
// endpoint, by detecting the endpoint address. Returns an error if it was not
// possible to detect the endpoint address.
func V1() (*v1.Client, error) {
	globalUtil.initV1.Do(func() {
		globalUtil.initRetryV1.SetupRetrier(&retry.Config{
			Name:          "ecsutil-meta-v1",
			AttemptMethod: initV1,
			Strategy:      retry.RetryCount,
			RetryCount:    10,
			RetryDelay:    30 * time.Second,
		})
	})
	if err := globalUtil.initRetryV1.TriggerRetry(); err != nil {
		log.Debugf("ECS metadata v1 client init error: %s", err)
		return nil, err
	}
	return globalUtil.v1, nil
}

// V2 returns a client for the ECS metadata API v2 that uses the default
// endpoint address.
func V2() *v2.Client {
	globalUtil.initV2.Do(func() {
		globalUtil.v2 = v2.NewDefaultClient()
	})
	return globalUtil.v2
}

// V3 returns a client for the ECS metadata API v3 by detecting the endpoint
// address for the specified container. Returns an error if it was not possible
// to detect the endpoint address.
func V3(containerID string) (*v3.Client, error) {
	return newClientV3ForContainer(containerID)
}

// V3FromCurrentTask returns a client for the ECS metadata API v3 by detedting
// the endpoint address from the task the executable is running in. Returns an
// error if it was not possible to detect the endpoint address.
func V3FromCurrentTask() (*v3.Client, error) {
	globalUtil.initV3.Do(func() {
		globalUtil.initRetryV3.SetupRetrier(&retry.Config{
			Name:          "ecsutil-meta-v3",
			AttemptMethod: initV3,
			Strategy:      retry.RetryCount,
			RetryCount:    10,
			RetryDelay:    30 * time.Second,
		})
	})
	if err := globalUtil.initRetryV3.TriggerRetry(); err != nil {
		log.Debugf("ECS metadata v3 client init error: %s", err)
		return nil, err
	}
	return globalUtil.v3, nil
}

// newAutodetectedClientV1 detects the metadata v1 API endpoint and creates a new
// client for it. Returns an error if it was not possible to find the endpoint.
func newAutodetectedClientV1() (*v1.Client, error) {
	agentURL, err := detectAgentV1URL()
	if err != nil {
		return nil, err
	}
	return v1.NewClient(agentURL), nil
}

// newClientV3ForContainer detects the metadata API v3 endpoint for the specified
// container and creates a new client for it.
func newClientV3ForContainer(id string) (*v3.Client, error) {
	agentURL, err := getAgentV3URLFromDocker(id)
	if err != nil {
		return nil, err
	}
	return v3.NewClient(agentURL), nil
}

// newClientV3ForCurrentTask detects the metadata API v3 endpoint from the current
// task and creates a new client for it.
func newClientV3ForCurrentTask() (*v3.Client, error) {
	agentURL, err := getAgentV3URLFromEnv()
	if err != nil {
		return nil, err
	}
	return v3.NewClient(agentURL), nil
}

func initV1() error {
	client, err := newAutodetectedClientV1()
	if err != nil {
		return err
	}
	globalUtil.v1 = client
	return nil
}

func initV3() error {
	client, err := newClientV3ForCurrentTask()
	if err != nil {
		return err
	}
	globalUtil.v3 = client
	return nil
}
