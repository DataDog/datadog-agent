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
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	globalConfigs   sync.Map
	globalConfigsMu sync.Mutex
)

type confKey struct {
	role   types.CloudID
	region string
}

// GetConfigFromCloudID returns an AWS Config for the given region and assumed role.
func GetConfigFromCloudID(ctx context.Context, roles types.RolesMapping, cloudID types.CloudID) aws.Config {
	return GetConfig(ctx, cloudID.Region(), roles.GetCloudIDRole(cloudID))
}

// GetConfig returns an AWS Config for the given region and assumed role.
func GetConfig(ctx context.Context, region string, assumedRole types.CloudID) aws.Config {
	key := confKey{assumedRole, region}
	if cfg, ok := globalConfigs.Load(key); ok {
		return cfg.(aws.Config)
	}

	globalConfigsMu.Lock()
	defer globalConfigsMu.Unlock()
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

	noDelegateCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		log.Errorf("awsconfig: could not load default config: %v", err)
		return aws.Config{}
	}

	var delegateCfg aws.Config
	stsclient := sts.NewFromConfig(noDelegateCfg)
	_, err = stsclient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(assumedRole.AsText()),
		RoleSessionName: aws.String("agentless-scanner"),
	})
	if err != nil {
		// In case we cannot assume the role they're maybe is a configuration
		// issue on IAM. However we try and fallback on using the default role
		// for the identity, instead of a delegate. This may be possible if
		// the user has attached the proper permissions directly on the
		// instance's role for instance.
		log.Warnf("awsconfig: could not assumerole %q: %v", assumedRole, err)
		delegateCfg = noDelegateCfg
		identity, err := stsclient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			log.Errorf("awsconfig: could not get caller identity: %v", err)
			return aws.Config{}
		}
		identityRole, err := types.ParseCloudID(*identity.Arn, types.ResourceTypeRole)
		if err != nil {
			log.Errorf("awsconfig: could not parse identity role: %v", err)
			return aws.Config{}
		}
		delegateCfg.HTTPClient = newHTTPClientWithStats(region, identityRole, statsd, limiter, tags)
		log.Warnf("awsconfig: fallbacking on default identity %q", *identity.Arn)
	} else {
		// We were able to check that the role is assumable, so we can use it.
		// Everything should be properly setup.
		stsassume := stscreds.NewAssumeRoleProvider(stsclient, assumedRole.AsText())
		delegateCfg, err = config.LoadDefaultConfig(ctx,
			config.WithHTTPClient(newHTTPClientWithStats(region, assumedRole, statsd, limiter, tags)),
			config.WithRegion(region),
			config.WithCredentialsProvider(aws.NewCredentialsCache(stsassume)))
		if err != nil {
			log.Errorf("awsconfig: could not load delegate config: %v", err)
		}
	}

	globalConfigs.Store(key, delegateCfg)
	return delegateCfg
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
