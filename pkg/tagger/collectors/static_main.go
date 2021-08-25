// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	staticCollectorName = "static"
)

// StaticCollector fetches "static" tags, e.g. those from an env var
// It is not intended to run as a stand alone. This is currently only used
// for Fargate instances where host tags are not collected
type StaticCollector struct {
	infoOut chan<- []*TagInfo
	tags    []string
}

// Detect detects static tags
func (c *StaticCollector) Detect(_ context.Context, out chan<- []*TagInfo) (CollectionMode, error) {
	c.infoOut = out
	c.tags = fargateStaticTags()

	return FetchOnlyCollection, nil
}

// Fetch fetches static tags
func (c *StaticCollector) Fetch(_ context.Context, entity string) ([]string, []string, []string, error) {
	tagInfoList := c.getTagInfo(entity)

	c.infoOut <- tagInfoList

	return c.tags, []string{}, []string{}, nil
}

func staticFactory() Collector {
	return &StaticCollector{}
}

func fargateStaticTags() (tags []string) {
	// Extract common ECS and EKS Fargate tags
	//   - DD_TAGS
	tags = append(tags, config.GetConfiguredTags(false)...)

	// Extract EKS Fargate specific tags
	//   - eks_fargate_node
	if fargate.IsEKSFargateInstance() {
		node, err := fargate.GetEKSFargateNodename()
		if err != nil {
			log.Infof("Couldn't build the 'eks_fargate_node' tag: %w", err)
			return
		}

		tags = append(tags, "eks_fargate_node:"+node)
	}

	return
}

func init() {
	// Only register collector if it is an ECS Fargate or EKS Fargate instance
	if fargate.IsFargateInstance(context.TODO()) {
		registerCollector(staticCollectorName, staticFactory, NodeOrchestrator)
	}
}
