// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"os"

	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/util/fargate"
)

// Azure Container Apps env var names, mirroring cmd/serverless-init/cloudservice/containerapp.go
// (redeclared locally rather than imported, since that package pulls in the full
// serverless-init cloud service bootstrap, not just these constants).
const (
	containerAppNameEnvVar        = "CONTAINER_APP_NAME"
	containerAppReplicaNameEnvVar = "CONTAINER_APP_REPLICA_NAME"
	azureSubscriptionIDEnvVar     = "DD_AZURE_SUBSCRIPTION_ID"
	azureResourceGroupEnvVar      = "DD_AZURE_RESOURCE_GROUP"
)

func isAzureContainerApps() bool {
	_, exists := os.LookupEnv(containerAppNameEnvVar)
	return exists
}

// acaIdentity holds the Azure Container Apps identifying attributes read from
// environment variables. name/subscriptionID/resourceGroup are the identity;
// replica is optional metadata.
type acaIdentity struct {
	replica        string
	name           string
	subscriptionID string
	resourceGroup  string
}

// workloadIdentity identifies which (if any) workload environment otel-agent is
// running in, so that the otel.ddot_collector.metrics.running billing metric can be
// tagged with the workload's identity instead of a hostname.
type workloadIdentity struct {
	// fargateTaskARN is non-empty when otel-agent is running as an ECS Fargate task
	// and the task ARN was fetched successfully.
	fargateTaskARN string
	// aca is non-nil when otel-agent is running as an Azure Container Apps replica.
	aca *acaIdentity
}

// detectWorkloadIdentity checks whether otel-agent is running on ECS Fargate or
// Azure Container Apps. It performs a network call for ECS Fargate (the ECS Task
// Metadata Endpoint v4), so it should be called at most once per metrics export
// cycle, not per metric.
func detectWorkloadIdentity(ctx context.Context, logger *zap.Logger) workloadIdentity {
	switch {
	case fargate.GetOrchestrator() == fargate.ECS:
		taskARN, err := fetchECSTaskARN(ctx)
		if err != nil {
			logger.Warn("failed to fetch ECS task ARN; falling back to host-based running metric", zap.Error(err))
			return workloadIdentity{}
		}
		return workloadIdentity{fargateTaskARN: taskARN}
	case isAzureContainerApps():
		return workloadIdentity{
			aca: &acaIdentity{
				replica:        os.Getenv(containerAppReplicaNameEnvVar),
				name:           os.Getenv(containerAppNameEnvVar),
				subscriptionID: os.Getenv(azureSubscriptionIDEnvVar),
				resourceGroup:  os.Getenv(azureResourceGroupEnvVar),
			},
		}
	default:
		return workloadIdentity{}
	}
}
