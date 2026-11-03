// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

const (
	ecsTaskDefinitionResourceType = "aws:ecs/taskDefinition:TaskDefinition"
	ecsTaskDefinitionDeleteBatch  = 10
	ecsTaskDefinitionCleanupTime  = 2 * time.Minute
)

type ecsTaskDefinitionDeleter interface {
	DeleteTaskDefinitions(context.Context, *ecs.DeleteTaskDefinitionsInput, ...func(*ecs.Options)) (*ecs.DeleteTaskDefinitionsOutput, error)
}

var newECSTaskDefinitionDeleter = func(ctx context.Context, region string) (ecsTaskDefinitionDeleter, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return ecs.NewFromConfig(cfg), nil
}

type pulumiDeployment struct {
	Resources []pulumiResource `json:"resources"`
}

type pulumiResource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

func captureECSTaskDefinitions(ctx context.Context, stack *auto.Stack) ([]string, error) {
	deployment, err := stack.Export(ctx)
	if err != nil {
		return nil, fmt.Errorf("export Pulumi stack: %w", err)
	}
	return ecsTaskDefinitionsFromDeployment(deployment.Deployment)
}

func ecsTaskDefinitionsFromDeployment(deployment json.RawMessage) ([]string, error) {
	var state pulumiDeployment
	if err := json.Unmarshal(deployment, &state); err != nil {
		return nil, fmt.Errorf("decode Pulumi stack: %w", err)
	}

	arns := make([]string, 0)
	for _, resource := range state.Resources {
		if resource.Type == ecsTaskDefinitionResourceType && resource.ID != "" {
			arns = append(arns, resource.ID)
		}
	}
	slices.Sort(arns)
	return slices.Compact(arns), nil
}

func deleteECSTaskDefinitions(ctx context.Context, arns []string, logger io.Writer) {
	byRegion := make(map[string][]string)
	for _, taskDefinitionARN := range arns {
		parts := strings.SplitN(taskDefinitionARN, ":", 6)
		if len(parts) != 6 || parts[0] != "arn" || parts[2] != "ecs" || parts[3] == "" {
			fmt.Fprintf(logger, "[WARNING] Cannot delete malformed ECS task definition ARN %q\n", taskDefinitionARN)
			continue
		}
		byRegion[parts[3]] = append(byRegion[parts[3]], taskDefinitionARN)
	}

	cleanupCtx, cancel := context.WithTimeout(ctx, ecsTaskDefinitionCleanupTime)
	defer cancel()
	for region, regionalARNs := range byRegion {
		client, err := newECSTaskDefinitionDeleter(cleanupCtx, region)
		if err != nil {
			fmt.Fprintf(logger, "[WARNING] Cannot create ECS client for task definition cleanup in %s: %v\n", region, err)
			continue
		}
		for start := 0; start < len(regionalARNs); start += ecsTaskDefinitionDeleteBatch {
			end := min(start+ecsTaskDefinitionDeleteBatch, len(regionalARNs))
			output, err := client.DeleteTaskDefinitions(cleanupCtx, &ecs.DeleteTaskDefinitionsInput{
				TaskDefinitions: regionalARNs[start:end],
			})
			if err != nil {
				fmt.Fprintf(logger, "[WARNING] Cannot delete ECS task definitions after stack destroy: %v\n", err)
				continue
			}
			for _, failure := range output.Failures {
				fmt.Fprintf(logger, "[WARNING] Cannot delete ECS task definition %s: %s %s\n", stringValue(failure.Arn), stringValue(failure.Reason), stringValue(failure.Detail))
			}
		}
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
