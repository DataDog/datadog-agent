// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ec2

package tags

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	ec2internal "github.com/DataDog/datadog-agent/pkg/util/ec2/internal"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// declare these as vars not const to ease testing
var (
	tagsCacheKey = cache.BuildAgentKey("ec2", "GetTags")
	infoCacheKey = cache.BuildAgentKey("ec2", "GetInstanceInfo")

	imdsTags = "/tags/instance"

	// for testing purposes
	fetchContainerInstanceARN = getContainerInstanceARN
)

func isTagExcluded(tag string) bool {
	if excludedTags := pkgconfigsetup.Datadog().GetStringSlice("exclude_ec2_tags"); excludedTags != nil {
		if slices.Contains(excludedTags, tag) {
			return true
		}
	}
	return false
}

// GetInstanceInfo collects information about the EC2 instance as host tags. This mimic the tags set by the AWS
// integration in Datadog backend allowing customer to collect those information without having to enable the crawler.
func GetInstanceInfo(ctx context.Context) ([]string, error) {
	if !configutils.IsCloudProviderEnabled(ec2internal.CloudProviderName, pkgconfigsetup.Datadog()) {
		return nil, errors.New("cloud provider is disabled by configuration")
	}

	if !pkgconfigsetup.Datadog().GetBool("collect_ec2_instance_info") {
		return nil, nil
	}

	if ec2Info, found := cache.Cache.Get(infoCacheKey); found {
		return ec2Info.([]string), nil
	}

	info, err := ec2internal.GetInstanceDocument(ctx)
	if err != nil {
		log.Debugf("could not fetch instance information: %s", err)
		return nil, err
	}

	tags := []string{}
	getAndSet := func(infoName string, tagName string) {
		if isTagExcluded(tagName) {
			return
		}
		if val, ok := info[infoName]; ok {
			tags = append(tags, fmt.Sprintf("%s:%s", tagName, val))
		} else {
			tags = append(tags, tagName+":unavailable")
		}
	}

	getAndSet("region", "region")
	getAndSet("instanceType", "instance-type")
	getAndSet("accountId", "aws_account")
	getAndSet("imageId", "image")
	getAndSet("availabilityZone", "availability-zone")

	// Add container instance ARN when running on ECS EC2
	if env.IsFeaturePresent(env.ECSEC2) {
		const ciaTagName = "container_instance_arn"
		if !isTagExcluded(ciaTagName) {
			arn, err := fetchContainerInstanceARN(ctx)
			if err != nil || arn == "" {
				log.Debugf("could not fetch container instance ARN: %v", err)
			} else {
				tags = append(tags, fmt.Sprintf("%s:%s", ciaTagName, arn))
			}
		}
	}

	// save tags to the cache in case we exceed quotas later
	cache.Cache.Set(infoCacheKey, tags, cache.NoExpiration)
	return tags, nil
}

func fetchEc2Tags(ctx context.Context) ([]string, error) {
	if pkgconfigsetup.Datadog().GetBool("collect_ec2_tags_use_imds") {
		// prefer to fetch tags from IMDS, falling back to the API
		tags, err := fetchEc2TagsFromIMDS(ctx)
		if err == nil {
			return tags, nil
		}

		log.Debugf("Could not fetch tags from instance metadata (trying EC2 API instead): %s", err)
	}

	return fetchEc2TagsFromAPI(ctx)
}

func fetchEc2TagsFromIMDS(ctx context.Context) ([]string, error) {
	keysStr, err := ec2internal.GetMetadataItem(ctx, imdsTags, ec2internal.UseIMDSv2(), true)
	if err != nil {
		return nil, err
	}

	// keysStr is a newline-separated list of strings containing tag keys
	keys := strings.Split(keysStr, "\n")

	tags := make([]string, 0, len(keys))
	for _, key := range keys {
		// The key is a valid URL component and need not be escaped:
		//
		// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/Using_Tags.html#tag-restrictions
		// > If you enable instance tags in instance metadata, instance tag
		// > keys can only use letters (a-z, A-Z), numbers (0-9), and the
		// > following characters: -_+=,.@:. Instance tag keys can't use spaces,
		// > /, or the reserved names ., .., or _index.
		val, err := ec2internal.GetMetadataItem(ctx, imdsTags+"/"+key, ec2internal.UseIMDSv2(), true)
		if err != nil {
			return nil, err
		}
		if isTagExcluded(key) {
			continue
		}

		tags = append(tags, fmt.Sprintf("%s:%s", key, val))
	}

	return tags, nil
}

func createEC2Client(ctx context.Context, region string, creds aws.CredentialsProvider) (*ec2.Client, error) {
	opts := []func(*config.LoadOptions) error{config.WithRegion(region)}
	if creds != nil {
		opts = append(opts, config.WithCredentialsProvider(creds))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}
	return ec2.NewFromConfig(cfg), nil
}

func fetchEc2TagsFromAPI(ctx context.Context) ([]string, error) {
	instanceIdentity, err := ec2internal.GetInstanceIdentity(ctx)
	if err != nil {
		return nil, err
	}

	// default client chain (IRSA/ECS/env/instance-profile chain)
	ec2Client, err := createEC2ClientFunc(ctx, instanceIdentity.Region, nil)
	if err != nil {
		log.Debugf("unable to create EC2 client with default credentials (falling back to instance role): %s", err)

		// If the above fails, for backward compatibility, fall back to our legacy
		// behavior, where we explicitly query instance role to get credentials.
		iamParams, err := getSecurityCreds(ctx)
		if err != nil {
			return nil, err
		}

		awsCreds := credentials.NewStaticCredentialsProvider(iamParams.AccessKeyID, iamParams.SecretAccessKey, iamParams.Token)
		// legacy client
		ec2Client, err = createEC2ClientFunc(ctx, instanceIdentity.Region, awsCreds)
		if err != nil {
			return nil, err
		}
	}

	return getTagsWithClientFunc(ctx, ec2Client, instanceIdentity)
}

func getTagsWithClient(ctx context.Context, client *ec2.Client, instanceIdentity *ec2internal.EC2Identity) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, pkgconfigsetup.Datadog().GetDuration("ec2_metadata_timeout")*time.Millisecond)
	defer cancel()

	describeTagsOutput, err := client.DescribeTags(ctx,
		&ec2.DescribeTagsInput{
			Filters: []types.Filter{{
				Name: aws.String("resource-id"),
				Values: []string{
					instanceIdentity.InstanceID,
				},
			}},
		},
	)

	if err != nil {
		return nil, err
	}

	tags := []string{}
	for _, tag := range describeTagsOutput.Tags {
		if isTagExcluded(*tag.Key) {
			continue
		}
		tags = append(tags, fmt.Sprintf("%s:%s", *tag.Key, *tag.Value))
	}
	return tags, nil
}

// for testing purposes
var fetchTags = fetchEc2Tags
var getTagsWithClientFunc = getTagsWithClient
var createEC2ClientFunc = createEC2Client

func fetchTagsFromCache(ctx context.Context) ([]string, error) {
	if !configutils.IsCloudProviderEnabled(ec2internal.CloudProviderName, pkgconfigsetup.Datadog()) {
		return nil, errors.New("cloud provider is disabled by configuration")
	}

	tags, err := fetchTags(ctx)
	if err != nil {
		if ec2Tags, found := cache.Cache.Get(tagsCacheKey); found {
			log.Infof("unable to get tags from aws, returning cached tags: %s", err)
			return ec2Tags.([]string), nil
		}
		return nil, fmt.Errorf("unable to get tags from aws and cache is empty: %s", err)
	}

	// save tags to the cache in case we exceed quotas later
	cache.Cache.Set(tagsCacheKey, tags, cache.NoExpiration)

	return tags, nil
}

// GetTags grabs the host tags from the EC2 api
func GetTags(ctx context.Context) ([]string, error) {
	tags, err := fetchTagsFromCache(ctx)
	if err != nil {
		log.Warn(err.Error())
	}
	return tags, err
}

type ec2SecurityCred struct {
	AccessKeyID     string
	SecretAccessKey string
	Token           string
}

func getSecurityCreds(ctx context.Context) (*ec2SecurityCred, error) {
	iamParams := &ec2SecurityCred{}

	iamRole, err := getIAMRole(ctx)
	if err != nil {
		return iamParams, err
	}

	res, err := ec2internal.DoHTTPRequest(ctx, ec2internal.MetadataURL+"/iam/security-credentials/"+iamRole, ec2internal.UseIMDSv2(), true)
	if err != nil {
		return iamParams, fmt.Errorf("unable to fetch EC2 API to get iam role: %s", err)
	}

	err = json.Unmarshal([]byte(res), &iamParams)
	if err != nil {
		return iamParams, fmt.Errorf("unable to unmarshall json, %s", err)
	}
	return iamParams, nil
}

func getIAMRole(ctx context.Context) (string, error) {
	res, err := ec2internal.DoHTTPRequest(ctx, ec2internal.MetadataURL+"/iam/security-credentials/", ec2internal.UseIMDSv2(), true)
	if err != nil {
		return "", fmt.Errorf("unable to fetch EC2 API to get security credentials: %s", err)
	}

	return res, nil
}
