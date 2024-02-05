// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package awsutils

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"
	"github.com/DataDog/datadog-agent/pkg/version"
	"golang.org/x/time/rate"

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

var (
	statsd *ddogstatsd.Client

	globalConfigs   = make(map[confKey]*aws.Config)
	globalConfigsMu sync.Mutex
)

type confKey struct {
	role   types.CloudID
	region string
}

// GetConfig returns an AWS Config for the given region and assumed role.
func GetConfig(ctx context.Context, region string, assumedRole *types.CloudID) (aws.Config, error) {
	globalConfigsMu.Lock()
	defer globalConfigsMu.Unlock()

	key := confKey{
		region: region,
	}
	if assumedRole != nil {
		key.role = *assumedRole
	}
	if cfg, ok := globalConfigs[key]; ok {
		return *cfg, nil
	}

	if statsd == nil {
		statsd, _ = ddogstatsd.New("localhost:8125")
	}
	tags := []string{
		fmt.Sprintf("agent_version:%s", version.AgentVersion),
	}
	limiter := NewLimiter(LimiterOptions{
		EC2Rate:          rate.Limit(pkgconfig.Datadog.GetFloat64("agentless_scanner.limits.aws_ec2_rate")),
		EBSListBlockRate: rate.Limit(pkgconfig.Datadog.GetFloat64("agentless_scanner.limits.aws_ebs_list_block_rate")),
		EBSGetBlockRate:  rate.Limit(pkgconfig.Datadog.GetFloat64("agentless_scanner.limits.aws_ebs_get_block_rate")),
		DefaultRate:      rate.Limit(pkgconfig.Datadog.GetFloat64("agentless_scanner.limits.aws_default_rate")),
	})
	httpClient := newHTTPClientWithStats(region, assumedRole, statsd, limiter, tags)
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithHTTPClient(httpClient),
	)
	if err != nil {
		return aws.Config{}, fmt.Errorf("awsconfig: could not load default config: %w", err)
	}

	stsclient := sts.NewFromConfig(cfg)
	if assumedRole != nil {
		stsassume := stscreds.NewAssumeRoleProvider(stsclient, assumedRole.String())
		cfg.Credentials = aws.NewCredentialsCache(stsassume)
	}

	identity, err := stsclient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return aws.Config{}, fmt.Errorf("awsconfig: could not assumerole %q: %w", assumedRole, err)
	}

	if assumedRole == nil {
		roleID, err := types.ParseCloudID(*identity.Arn, types.ResourceTypeRole)
		if err != nil {
			return aws.Config{}, fmt.Errorf("awsconfig: could not parse caller identity arn: %w", err)
		}
		cfg.HTTPClient = newHTTPClientWithStats(region, &roleID, statsd, limiter, tags)
	}

	globalConfigs[key] = &cfg
	return cfg, nil
}

// GetSelfEC2InstanceIndentity returns the identity of the current EC2 instance.
func GetSelfEC2InstanceIndentity(ctx context.Context) (*imds.GetInstanceIdentityDocumentOutput, error) {
	// TODO: we could cache this information instead of polling imds every time
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	imdsclient := imds.NewFromConfig(cfg)
	return imdsclient.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
}
