// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package awsutils

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

var (
	statsd *ddogstatsd.Client

	globalConfigs        = make(map[confKey]*aws.Config)
	globalConfigsMu      sync.Mutex
	globalStatsTags      []string
	globalLimiterOptions LimiterOptions
)

type confKey struct {
	role   types.CloudID
	region string
}

// InitConfig initializes the global AWS parameters for subsequent configs
// with the given statsd client and tags.
func InitConfig(client *ddogstatsd.Client, limiterOptions LimiterOptions, tags []string) {
	statsd = client
	globalStatsTags = tags
	globalLimiterOptions = limiterOptions
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

	limiter := NewLimiter(globalLimiterOptions)
	httpClient := newHTTPClientWithStats(region, assumedRole, statsd, limiter, globalStatsTags)
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
		cfg.HTTPClient = newHTTPClientWithStats(region, &roleID, statsd, limiter, globalStatsTags)
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

// HumanParseARN parses an ARN string or a resource identifier string and
// returns an cloud identifier. Helpful for CLI interface.
func HumanParseARN(s string, expectedTypes ...types.ResourceType) (types.CloudID, error) {
	if strings.HasPrefix(s, "arn:") {
		return types.ParseCloudID(s, expectedTypes...)
	}
	self, err := GetSelfEC2InstanceIndentity(context.Background())
	if err != nil {
		return types.CloudID{}, err
	}
	partition := "aws"
	var service string
	if strings.HasPrefix(s, "/") && (len(s) == 1 || fs.ValidPath(s[1:])) {
		partition = "localhost"
	} else if strings.HasPrefix(s, "vol-") {
		service = "ec2"
		s = "volume/" + s
	} else if strings.HasPrefix(s, "snap-") {
		service = "ec2"
		s = "snapshot/" + s
	} else if strings.HasPrefix(s, "function:") {
		service = "lambda"
	} else {
		return types.CloudID{}, fmt.Errorf("unable to parse resource: expecting a resource of types %v", expectedTypes)
	}
	arn := fmt.Sprintf("arn:%s:%s:%s:%s:%s", partition, service, self.Region, self.AccountID, s)
	return types.ParseCloudID(arn, expectedTypes...)
}
