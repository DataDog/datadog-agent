// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pool

import (
	"context"
	"fmt"

	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	awsssm "github.com/aws/aws-sdk-go-v2/service/ssm"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

// macOSDefaultVersion mirrors scenarios/aws/ec2/os_resolver.go's resolveMacosAMI default,
// applied here without a *pulumi.Context so the macOS pool provisioner can resolve an AMI
// without depending on Pulumi at all.
var macOSDefaultVersion = e2eos.MacOSSonoma.Version

// ResolveMacOSAMI returns the AMI ID for osInfo, defaulting Version to macOSDefaultVersion
// when unset, via a direct SSM GetParameter call (the same public AWS-maintained parameter
// path resolveMacosAMI reads through a Pulumi data source).
func ResolveMacOSAMI(ctx context.Context, region, profile string, osInfo e2eos.Descriptor) (string, error) {
	version := osInfo.Version
	if version == "" {
		version = macOSDefaultVersion
	}

	cfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(region),
		awsConfig.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config to resolve macOS AMI: %w", err)
	}

	paramName := fmt.Sprintf("/aws/service/ec2-macos/%s/%s_mac/latest/image_id", version, osInfo.Architecture)
	out, err := awsssm.NewFromConfig(cfg).GetParameter(ctx, &awsssm.GetParameterInput{Name: &paramName})
	if err != nil {
		return "", fmt.Errorf("failed to resolve macOS AMI from SSM parameter %s: %w", paramName, err)
	}
	if out.Parameter == nil || out.Parameter.Value == nil {
		return "", fmt.Errorf("SSM parameter %s returned no value", paramName)
	}
	return *out.Parameter.Value, nil
}
