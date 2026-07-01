// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"maps"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
)

// MicroVMOrigin origin tag value
const MicroVMOrigin = "lambda-microvm"

const (
	microVMPrefix = "aws.lambda.microvm."

	microVMUsageMetricSuffix = "instance"
)

// MicroVM implements CloudService for AWS Lambda MicroVMs.
type MicroVM struct{}

// GetTags returns MicroVM-specific tags parsed from the image ARN env var.
func (m *MicroVM) GetTags() map[string]string {
	tags := map[string]string{
		"origin":     MicroVMOrigin,
		"_dd.origin": MicroVMOrigin,
	}

	arn := os.Getenv(serverlessenv.MicroVMImageARNEnvVar)
	if arn == "" {
		tags["region"] = "unknown"
		tags["account_id"] = "unknown"
		tags["image_name"] = "unknown"
		return tags
	}

	region, accountID, imageName := parseMicroVMARN(arn)
	tags["region"] = region
	tags["account_id"] = accountID
	tags["image_name"] = imageName

	return tags
}

// GetEnhancedMetricTags returns base (low-cardinality) and usage tags.
func (m *MicroVM) GetEnhancedMetricTags(tags map[string]string) EnhancedMetricTags {
	baseTags := map[string]string{
		"account_id": tagValueOrUnknown(tags["account_id"]),
		"image_name": tagValueOrUnknown(tags["image_name"]),
		"origin":     tagValueOrUnknown(tags["origin"]),
		"region":     tagValueOrUnknown(tags["region"]),
	}
	return EnhancedMetricTags{Base: baseTags, Usage: maps.Clone(baseTags)}
}

// GetDefaultLogsSource returns the default logs source.
func (m *MicroVM) GetDefaultLogsSource() string { return MicroVMOrigin }

// GetMetricPrefix returns the AWS MicroVM metric prefix.
func (m *MicroVM) GetMetricPrefix() string { return microVMPrefix }

// GetUsageMetricSuffix returns the usage metric suffix.
func (m *MicroVM) GetUsageMetricSuffix() string { return microVMUsageMetricSuffix }

// GetOrigin returns the origin tag value.
func (m *MicroVM) GetOrigin() string { return MicroVMOrigin }

// GetSource returns the metrics source.
func (m *MicroVM) GetSource() metrics.MetricSource {
	return metrics.MetricSourceAWSMicroVMEnhanced
}

// Init is a no-op for MicroVM; lifecycle events are handled by the lifecycle server.
func (m *MicroVM) Init(_ *TracingContext) error { return nil }

// Shutdown is a no-op for MicroVM. The lifecycle server emits the terminate
// metric when the /terminate hook fires; this deferred cleanup must not
// double-emit it.
func (m *MicroVM) Shutdown(_ serverlessMetrics.ServerlessMetricAgent, _ bool, _ error) {}

// AddStartMetric is a no-op for MicroVM. The lifecycle server emits the launch
// metric when the /launch hook fires; emitting it here would double-count.
func (m *MicroVM) AddStartMetric(_ *serverlessMetrics.ServerlessMetricAgent) {}

// ShouldForceFlushAllOnForceFlushToSerializer returns false for MicroVM.
func (m *MicroVM) ShouldForceFlushAllOnForceFlushToSerializer() bool { return false }

// isMicroVM returns true when running inside an AWS Lambda MicroVM.
func isMicroVM() bool {
	_, exists := os.LookupEnv(serverlessenv.MicroVMImageARNEnvVar)
	return exists
}

// parseMicroVMARN extracts region, accountID, and imageName from an ARN of the
// form arn:aws:lambda:<region>:<account>:microvm-image:<name>.
// Returns "unknown" for any field that cannot be parsed.
func parseMicroVMARN(arn string) (region, accountID, imageName string) {
	parts := strings.Split(arn, ":")
	region = "unknown"
	accountID = "unknown"
	imageName = "unknown"
	// ARN format: arn:aws:lambda:region:account:microvm-image:name
	if len(parts) >= 5 {
		if parts[3] != "" {
			region = parts[3]
		}
		if parts[4] != "" {
			accountID = parts[4]
		}
	}
	if len(parts) >= 7 && parts[6] != "" {
		imageName = strings.Join(parts[6:], ":")
	}
	return region, accountID, imageName
}
