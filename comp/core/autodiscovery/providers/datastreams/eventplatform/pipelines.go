// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatform contains Data Streams event-platform pipeline descriptors.
package eventplatform

import (
	"context"
	"fmt"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const EventTypeDataStreamsMessage = "data-streams-message"

// Pipelines returns the Data Streams event-platform pipelines.
func Pipelines() []eventplatform.PipelineDesc {
	return []eventplatform.PipelineDesc{
		{
			EventType:                     EventTypeDataStreamsMessage,
			Category:                      "Data Streams",
			ContentType:                   eventplatform.ContentTypeJSON,
			EndpointsConfigPrefix:         "data_streams.forwarder.",
			HostnameEndpointPrefix:        "trace.agent.",
			IntakeTrackType:               "data_streams_messages",
			DefaultBatchMaxConcurrentSend: 10,
			DefaultBatchMaxContentSize:    eventplatform.DefaultBatchMaxContentSize,
			DefaultBatchMaxSize:           eventplatform.DefaultBatchMaxSize,
			DefaultInputChanSize:          eventplatform.DefaultInputChanSize,
			ExtraHTTPHeaders:              extraHTTPHeaders,
		},
	}
}

func extraHTTPHeaders(hostname string) map[string]string {
	tags := fmt.Sprintf("host:%s,agent_version:%s", hostname, version.AgentVersion)
	if taskARN := getECSFargateTaskARN(); taskARN != "" {
		tags += ",task_arn:" + taskARN
	}
	return map[string]string{
		"X-Datadog-Additional-Tags": tags,
	}
}

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
