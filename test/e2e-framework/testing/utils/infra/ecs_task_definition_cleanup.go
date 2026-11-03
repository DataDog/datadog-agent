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

var newECSTaskDefinitionDeleter = func(ctx context.Context, region, profile string) (ecsTaskDefinitionDeleter, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if profile != "" {
		loadOptions = append(loadOptions, awsconfig.WithSharedConfigProfile(profile))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, err
	}
	return ecs.NewFromConfig(cfg), nil
}

type pulumiDeployment struct {
	Resources []pulumiResource `json:"resources"`
}

type pulumiResource struct {
	URN      string                     `json:"urn"`
	Type     string                     `json:"type"`
	ID       string                     `json:"id"`
	Provider string                     `json:"provider"`
	Inputs   map[string]json.RawMessage `json:"inputs"`
}

type ecsTaskDefinition struct {
	ARN     string
	Profile string
}

func captureECSTaskDefinitions(ctx context.Context, stack *auto.Stack) ([]ecsTaskDefinition, error) {
	deployment, err := stack.Export(ctx)
	if err != nil {
		return nil, fmt.Errorf("export Pulumi stack: %w", err)
	}
	return ecsTaskDefinitionsFromDeployment(deployment.Deployment)
}

func ecsTaskDefinitionsFromDeployment(deployment json.RawMessage) ([]ecsTaskDefinition, error) {
	var state pulumiDeployment
	if err := json.Unmarshal(deployment, &state); err != nil {
		return nil, fmt.Errorf("decode Pulumi stack: %w", err)
	}

	providerProfiles := make(map[string]string)
	for _, resource := range state.Resources {
		if resource.Type != "pulumi:providers:aws" || resource.URN == "" || resource.ID == "" {
			continue
		}
		var profile string
		if rawProfile, found := resource.Inputs["profile"]; found {
			if err := json.Unmarshal(rawProfile, &profile); err != nil {
				return nil, fmt.Errorf("decode AWS provider profile: %w", err)
			}
		}
		providerProfiles[resource.URN+"::"+resource.ID] = profile
	}

	taskDefinitions := make([]ecsTaskDefinition, 0)
	for _, resource := range state.Resources {
		if resource.Type == ecsTaskDefinitionResourceType && resource.ID != "" {
			taskDefinitions = append(taskDefinitions, ecsTaskDefinition{
				ARN:     resource.ID,
				Profile: providerProfiles[resource.Provider],
			})
		}
	}
	slices.SortFunc(taskDefinitions, func(a, b ecsTaskDefinition) int {
		return strings.Compare(a.ARN, b.ARN)
	})
	return slices.CompactFunc(taskDefinitions, func(a, b ecsTaskDefinition) bool {
		return a.ARN == b.ARN
	}), nil
}

func deleteECSTaskDefinitions(ctx context.Context, taskDefinitions []ecsTaskDefinition, logger io.Writer) {
	type cleanupTarget struct {
		region  string
		profile string
	}
	byTarget := make(map[cleanupTarget][]string)
	for _, taskDefinition := range taskDefinitions {
		parts := strings.SplitN(taskDefinition.ARN, ":", 6)
		if len(parts) != 6 || parts[0] != "arn" || parts[2] != "ecs" || parts[3] == "" {
			fmt.Fprintf(logger, "[WARNING] Cannot delete malformed ECS task definition ARN %q\n", taskDefinition.ARN)
			continue
		}
		target := cleanupTarget{region: parts[3], profile: taskDefinition.Profile}
		byTarget[target] = append(byTarget[target], taskDefinition.ARN)
	}

	cleanupCtx, cancel := context.WithTimeout(ctx, ecsTaskDefinitionCleanupTime)
	defer cancel()
	for target, targetARNs := range byTarget {
		client, err := newECSTaskDefinitionDeleter(cleanupCtx, target.region, target.profile)
		if err != nil {
			fmt.Fprintf(logger, "[WARNING] Cannot create ECS client for task definition cleanup in %s: %v\n", target.region, err)
			continue
		}
		for start := 0; start < len(targetARNs); start += ecsTaskDefinitionDeleteBatch {
			end := min(start+ecsTaskDefinitionDeleteBatch, len(targetARNs))
			output, err := client.DeleteTaskDefinitions(cleanupCtx, &ecs.DeleteTaskDefinitionsInput{
				TaskDefinitions: targetARNs[start:end],
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
