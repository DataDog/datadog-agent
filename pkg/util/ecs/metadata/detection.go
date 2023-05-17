// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build docker

package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/system"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	v3or4 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
)

func detectAgentV1URL() (string, error) {
	urls := make([]string, 0, 3)

	if len(config.Datadog.GetString("ecs_agent_url")) > 0 {
		urls = append(urls, config.Datadog.GetString("ecs_agent_url"))
	}

	if config.IsContainerized() {
		// List all interfaces for the ecs-agent container
		agentURLS, err := getAgentV1ContainerURLs(context.TODO())
		if err != nil {
			log.Debugf("Could not inspect ecs-agent container: %s", err)
		} else {
			urls = append(urls, agentURLS...)
		}
		// Try the default gateway
		gw, err := system.GetDefaultGateway(config.Datadog.GetString("proc_root"))
		if err != nil {
			log.Debugf("Could not get docker default gateway: %s", err)
		}
		if gw != nil {
			urls = append(urls, fmt.Sprintf("http://%s:%d/", gw.String(), v1.DefaultAgentPort))
		}

		// Try the default IP for awsvpc mode
		urls = append(urls, fmt.Sprintf("http://169.254.172.1:%d/", v1.DefaultAgentPort))
	}

	// Always try the localhost URL.
	urls = append(urls, fmt.Sprintf("http://localhost:%d/", v1.DefaultAgentPort))

	detected := testURLs(urls, 1*time.Second)
	if detected != "" {
		return detected, nil
	}

	return "", fmt.Errorf("could not detect ECS agent, tried URLs: %s", urls)
}

func getAgentV1ContainerURLs(ctx context.Context) ([]string, error) {
	var urls []string

	if !config.IsFeaturePresent(config.Docker) {
		return nil, errors.New("Docker feature not activated")
	}

	du, err := docker.GetDockerUtil()
	if err != nil {
		return nil, err
	}
	ecsConfig, err := du.Inspect(ctx, config.Datadog.GetString("ecs_agent_container_name"), false)
	if err != nil {
		return nil, err
	}

	for _, network := range ecsConfig.NetworkSettings.Networks {
		ip := network.IPAddress
		if ip != "" {
			urls = append(urls, fmt.Sprintf("http://%s:%d/", ip, v1.DefaultAgentPort))
		}
	}

	// Add the container hostname, as it holds the instance's private IP when ecs-agent
	// runs in the (default) host network mode. This allows us to connect back to it
	// from an agent container running in awsvpc mode.
	if ecsConfig.Config != nil && ecsConfig.Config.Hostname != "" {
		urls = append(urls, fmt.Sprintf("http://%s:%d/", ecsConfig.Config.Hostname, v1.DefaultAgentPort))
	}

	return urls, nil
}

// testURLs trys a set of URLs and returns the first one that succeeds.
func testURLs(urls []string, timeout time.Duration) string {
	client := &http.Client{Timeout: timeout}
	for _, url := range urls {
		r, err := client.Get(url)
		if err != nil {
			continue
		}
		defer r.Body.Close()
		if r.StatusCode != http.StatusOK {
			continue
		}
		var resp v1.Commands
		if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
			log.Debugf("Error decoding JSON response from '%s': %s", url, err)
			continue
		}
		if len(resp.AvailableCommands) > 0 {
			return url
		}
	}
	return ""
}

func getAgentV3URLFromEnv() (string, error) {
	agentURL, found := os.LookupEnv(v3or4.DefaultMetadataURIv3EnvVariable)
	if !found {
		return "", fmt.Errorf("Could not initialize client: missing metadata v3 URL")
	}
	return agentURL, nil
}

func getAgentV4URLFromEnv() (string, error) {
	agentURL, found := os.LookupEnv(v3or4.DefaultMetadataURIv4EnvVariable)
	if !found {
		return "", fmt.Errorf("Could not initialize client: missing metadata v4 URL")
	}
	return agentURL, nil
}
