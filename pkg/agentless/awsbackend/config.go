// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package awsbackend

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"golang.org/x/time/rate"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	smithymiddleware "github.com/aws/smithy-go/middleware"
)

const (
	// DefaultSelfRegion is the default region for the self identity.
	//
	// TODO: we should have this as part of our configuration to be able to
	// bootstrap ourselves in the correct region.
	DefaultSelfRegion = "us-east-1"
)

var (
	globalConfigs   map[confKey]aws.Config
	globalConfigsMu sync.Mutex
)

type confKey struct {
	role   types.CloudID
	region string
}

func loadDefaultConfig(ctx context.Context, options ...func(*config.LoadOptions) error) aws.Config {
	cfg, err := config.LoadDefaultConfig(ctx, append([]func(*config.LoadOptions) error{
		config.WithAPIOptions([]func(*smithymiddleware.Stack) error{
			middleware.AddUserAgentKeyValue("DatadogAgentlessScanner", version.AgentVersion),
		}),
	}, options...)...)
	if err != nil {
		log.Errorf("awsconfig: could not load default config: %v", err)
		return aws.Config{}
	}
	return cfg
}

// GetConfigFromCloudID returns an AWS Config for the given region and assumed role.
func GetConfigFromCloudID(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, roles types.RolesMapping, cloudID types.CloudID) aws.Config {
	return GetConfig(ctx, statsd, sc, cloudID.Region(), roles.GetCloudIDRole(cloudID))
}

// GetConfig returns an AWS Config for the given region and assumed role.
func GetConfig(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, region string, assumedRole types.CloudID) aws.Config {
	globalConfigsMu.Lock()
	defer globalConfigsMu.Unlock()

	key := confKey{assumedRole, region}
	if cfg, ok := globalConfigs[key]; ok {
		return cfg
	}

	tags := []string{
		fmt.Sprintf("agent_version:%s", version.AgentVersion),
	}
	limiter := NewLimiter(LimiterOptions{
		EC2Rate:          rate.Limit(sc.AWSEC2Rate),
		EBSListBlockRate: rate.Limit(sc.AWSEBSListBlockRate),
		EBSGetBlockRate:  rate.Limit(sc.AWSEBSGetBlockRate),
		DefaultRate:      rate.Limit(sc.AWSDefaultRate),
	})

	noDelegateCfg := loadDefaultConfig(ctx, config.WithRegion(region))
	var delegateCfg aws.Config
	stsclient := sts.NewFromConfig(noDelegateCfg)
	_, err := stsclient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(assumedRole.AsText()),
		RoleSessionName: aws.String("DatadogAgentlessScanner"),
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
		stsassume := stscreds.NewAssumeRoleProvider(stsclient, assumedRole.AsText(), func(options *stscreds.AssumeRoleOptions) {
			options.RoleSessionName = "DatadogAgentlessScanner"
		})
		delegateCfg = loadDefaultConfig(ctx,
			config.WithHTTPClient(newHTTPClientWithStats(region, assumedRole, statsd, limiter, tags)),
			config.WithRegion(region),
			config.WithCredentialsProvider(aws.NewCredentialsCache(stsassume)))
	}

	if globalConfigs == nil {
		globalConfigs = make(map[confKey]aws.Config)
	}
	globalConfigs[key] = delegateCfg
	return delegateCfg
}

func getSelfEC2InstanceIndentity(ctx context.Context) (*imds.GetInstanceIdentityDocumentOutput, error) {
	// TODO: we could cache this information instead of polling imds every time
	imdsclient := imds.NewFromConfig(loadDefaultConfig(ctx))
	return imdsclient.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
}

// GetIdentity returns the identity of the current assumed role.
func GetIdentity(ctx context.Context) (*sts.GetCallerIdentityOutput, error) {
	stsclient := sts.NewFromConfig(loadDefaultConfig(ctx, config.WithRegion(DefaultSelfRegion)))
	return stsclient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
}
