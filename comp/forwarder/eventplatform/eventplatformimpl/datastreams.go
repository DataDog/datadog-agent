// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package eventplatformimpl

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func addDataStreamsMessageHeaders(endpoints *config.Endpoints, hostname string) {
	tags := fmt.Sprintf("host:%s,agent_version:%s", hostname, version.AgentVersion)
	if taskARN := getECSFargateTaskARN(); taskARN != "" {
		tags += ",task_arn:" + taskARN
	}
	extraHeaders := map[string]string{
		"X-Datadog-Additional-Tags": tags,
	}
	for i := range endpoints.Endpoints {
		endpoints.Endpoints[i].ExtraHTTPHeaders = extraHeaders
	}
}

// getECSFargateTaskARN returns the ECS task ARN when running on Fargate, or empty string otherwise.
func getECSFargateTaskARN() string {
	if !env.IsECSFargate() {
		return ""
	}
	client, err := ecsmeta.V2()
	if err != nil {
		log.Debugf("Failed to initialize ECS metadata V2 client for task ARN: %v", err)
		return ""
	}
	taskMeta, err := client.GetTask(context.Background())
	if err != nil {
		log.Debugf("Failed to get ECS task metadata for task ARN: %v", err)
		return ""
	}
	return taskMeta.TaskARN
}
