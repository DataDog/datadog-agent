// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package ecs

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	json "github.com/json-iterator/go"
)

const (
	// DefaultAgentPort is the default port used by the ECS Agent.
	DefaultAgentPort = 51678
	// DefaultECSContainer is the default container used by ECS.
	DefaultECSContainer = "ecs-agent"
	// Cache the fact we're running on ECS Fargate
	isFargateInstanceCacheKey = "IsFargateInstanceCacheKey"
)

type (
	// CommandsV1Response is the format of a response from the ECS-agent on the root.
	CommandsV1Response struct {
		AvailableCommands []string `json:"AvailableCommands"`
	}

	// TasksV1Response is the format of a response from the ECS tasks API.
	// See http://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-agent-introspection.html
	TasksV1Response struct {
		Tasks []TaskV1 `json:"tasks"`
	}

	// TaskV1 is the format of a Task in the ECS tasks API.
	TaskV1 struct {
		Arn           string        `json:"Arn"`
		DesiredStatus string        `json:"DesiredStatus"`
		KnownStatus   string        `json:"KnownStatus"`
		Family        string        `json:"Family"`
		Version       string        `json:"Version"`
		Containers    []ContainerV1 `json:"containers"`
	}

	// ContainerV1 is the format of a Container in the ECS tasks API.
	ContainerV1 struct {
		DockerID   string `json:"DockerId"`
		DockerName string `json:"DockerName"`
		Name       string `json:"Name"`
	}
)

var globalUtil *Util

// GetUtil returns a ready to use ecs Util. It is backed by a shared singleton.
func GetUtil() (*Util, error) {
	if globalUtil == nil {
		globalUtil = &Util{}
		globalUtil.initRetry.SetupRetrier(&retry.Config{
			Name:          "ecsutil",
			AttemptMethod: globalUtil.init,
			Strategy:      retry.RetryCount,
			RetryCount:    10,
			RetryDelay:    30 * time.Second,
		})
	}
	if err := globalUtil.initRetry.TriggerRetry(); err != nil {
		log.Debugf("ECS init error: %s", err)
		return nil, err
	}
	return globalUtil, nil
}

// init makes an empty Util bootstrap itself.
func (u *Util) init() error {
	url, err := detectAgentURL()
	if err != nil {
		return err
	}
	u.agentURL = url

	return nil
}

// IsFargateInstance returns whether the agent is in an ECS fargate task.
// It detects it by getting and unmarshalling the metadata API response.
func IsFargateInstance() bool {
	var ok, isFargate bool

	if cached, hit := cache.Cache.Get(isFargateInstanceCacheKey); hit {
		isFargate, ok = cached.(bool)
		if !ok {
			log.Errorf("Invalid fargate instance cache format, forcing a cache miss")
		} else {
			return isFargate
		}
	}

	client := &http.Client{Timeout: timeout}
	r, err := client.Get(metadataURL)
	if err != nil {
		cacheIsFargateInstance(false)
		return false
	}
	if r.StatusCode != http.StatusOK {
		cacheIsFargateInstance(false)
		return false
	}
	var resp TaskMetadata
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		fmt.Printf("decode err: %s\n", err)
		cacheIsFargateInstance(false)
		return false
	}

	cacheIsFargateInstance(true)
	return true
}

func cacheIsFargateInstance(isFargate bool) {
	cacheDuration := 5 * time.Minute
	if isFargate {
		cacheDuration = cache.NoExpiration
	}
	cache.Cache.Set(isFargateInstanceCacheKey, isFargate, cacheDuration)
}

// IsAgentNotDetected indicates if an error from GetTasks was about no
// ECS agent being detected. This is a used as a way to check if is available
// or if the host is not in an ECS cluster.
func IsAgentNotDetected(err error) bool {
	return strings.Contains(err.Error(), "could not detect ECS agent")
}

// GetTasks returns a TasksV1Response containing information about the state
// of the local ECS containers running on this node. This data is provided via
// the local ECS agent
func (u *Util) GetTasks() (TasksV1Response, error) {
	var resp TasksV1Response
	r, err := http.Get(fmt.Sprintf("%sv1/tasks", u.agentURL))
	if err != nil {
		return resp, err
	}
	defer r.Body.Close()

	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		return resp, err
	}
	return resp, nil
}

// detectAgentURL finds a hostname for the ECS-agent either via Docker, if
// running inside of a container, or just defaulting to localhost.
func detectAgentURL() (string, error) {
	urls := make([]string, 0, 3)

	if len(config.Datadog.GetString("ecs_agent_url")) > 0 {
		urls = append(urls, config.Datadog.GetString("ecs_agent_url"))
	}

	if config.IsContainerized() {
		du, err := docker.GetDockerUtil()
		if err != nil {
			return "", err
		}

		// Try all networks available on the ecs container.
		ecsConfig, err := du.Inspect(DefaultECSContainer, false)
		if err != nil {
			return "", err
		}

		for _, network := range ecsConfig.NetworkSettings.Networks {
			ip := network.IPAddress
			if ip != "" {
				urls = append(urls, fmt.Sprintf("http://%s:%d/", ip, DefaultAgentPort))
			}
		}

		// Try the default gateway
		gw, err := docker.DefaultGateway()
		if err != nil {
			// "expected" errors are handled in DefaultGateway so only
			// unexpected errors are bubbled up, so we keep bubbling.
			return "", err
		}
		if gw != nil {
			urls = append(urls, fmt.Sprintf("http://%s:%d/", gw.String(), DefaultAgentPort))
		}
	}

	// Always try the localhost URL.
	urls = append(urls, fmt.Sprintf("http://localhost:%d/", DefaultAgentPort))
	detected := testURLs(urls, 1*time.Second)
	if detected != "" {
		return detected, nil
	}
	return "", fmt.Errorf("could not detect ECS agent, tried URLs: %s", urls)
}

// testURLs trys a set of URLs and returns the first one that succeeds.
func testURLs(urls []string, timeout time.Duration) string {
	client := &http.Client{Timeout: timeout}
	for _, url := range urls {
		r, err := client.Get(url)
		if err != nil {
			continue
		}
		if r.StatusCode != http.StatusOK {
			continue
		}
		var resp CommandsV1Response
		if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
			fmt.Printf("decode err: %s\n", err)
			continue
		}
		if len(resp.AvailableCommands) > 0 {
			return url
		}
	}
	return ""
}
