// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build ec2

package ec2

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// declare these as vars not const to ease testing
var (
	instanceIdentityURL = "http://169.254.169.254/latest/dynamic/instance-identity/document/"
	tagsCacheKey        = cache.BuildAgentKey("ec2", "GetTags")
)

func fetchEc2Tags(ctx context.Context) ([]string, error) {
	instanceIdentity, err := getInstanceIdentity(ctx)
	if err != nil {
		return nil, err
	}

	// First, try automatic credentials detection. This works in most scenarios,
	// except when a more specific role (e.g. task role in ECS) does not have
	// EC2:DescribeTags permission, but a more general role (e.g. instance role)
	// does have it.
	tags, err := getTagsWithCreds(ctx, instanceIdentity, nil)
	if err == nil {
		return tags, nil
	}
	log.Warnf("unable to get tags using default credentials (falling back to instance role): %s", err)

	// If the above fails, for backward compatibility, fall back to our legacy
	// behavior, where we explicitly query instance role to get credentials.
	iamParams, err := getSecurityCreds(ctx)
	if err != nil {
		return nil, err
	}

	awsCreds := credentials.NewStaticCredentials(iamParams.AccessKeyID,
		iamParams.SecretAccessKey,
		iamParams.Token)

	return getTagsWithCreds(ctx, instanceIdentity, awsCreds)
}

func getTagsWithCreds(ctx context.Context, instanceIdentity *ec2Identity, awsCreds *credentials.Credentials) ([]string, error) {
	awsSess, err := session.NewSession(&aws.Config{
		Region:      aws.String(instanceIdentity.Region),
		Credentials: awsCreds,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to get aws session, %s", err)
	}

	connection := ec2.New(awsSess)
	ec2Tags, err := connection.DescribeTagsWithContext(ctx,
		&ec2.DescribeTagsInput{
			Filters: []*ec2.Filter{{
				Name: aws.String("resource-id"),
				Values: []*string{
					aws.String(instanceIdentity.InstanceID),
				},
			}},
		},
	)

	if err != nil {
		return nil, err
	}

	tags := []string{}
	for _, tag := range ec2Tags.Tags {
		tags = append(tags, fmt.Sprintf("%s:%s", *tag.Key, *tag.Value))
	}
	return tags, nil
}

// for testing purposes
var fetchTags = fetchEc2Tags

func fetchTagsFromCache(ctx context.Context) ([]string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return nil, fmt.Errorf("cloud provider is disabled by configuration")
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

type ec2Identity struct {
	Region     string
	InstanceID string
}

func getInstanceIdentity(ctx context.Context) (*ec2Identity, error) {
	instanceIdentity := &ec2Identity{}

	res, err := doHTTPRequest(ctx, instanceIdentityURL, http.MethodGet, map[string]string{}, config.Datadog.GetBool("ec2_prefer_imdsv2"))
	if err != nil {
		return instanceIdentity, fmt.Errorf("unable to fetch EC2 API, %s", err)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return instanceIdentity, fmt.Errorf("unable to read identity body, %s", err)
	}

	err = json.Unmarshal(all, &instanceIdentity)
	if err != nil {
		return instanceIdentity, fmt.Errorf("unable to unmarshall json, %s", err)
	}

	return instanceIdentity, nil
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

	res, err := doHTTPRequest(ctx, metadataURL+"/iam/security-credentials/"+iamRole, http.MethodGet, map[string]string{}, config.Datadog.GetBool("ec2_prefer_imdsv2"))
	if err != nil {
		return iamParams, fmt.Errorf("unable to fetch EC2 API, %s", err)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return iamParams, fmt.Errorf("unable to read iam role body, %s", err)
	}

	err = json.Unmarshal(all, &iamParams)
	if err != nil {
		return iamParams, fmt.Errorf("unable to unmarshall json, %s", err)
	}
	return iamParams, nil
}

func getIAMRole(ctx context.Context) (string, error) {
	res, err := doHTTPRequest(ctx, metadataURL+"/iam/security-credentials/", http.MethodGet, map[string]string{}, config.Datadog.GetBool("ec2_prefer_imdsv2"))
	if err != nil {
		return "", fmt.Errorf("unable to fetch EC2 API, %s", err)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read security credentials body, %s", err)
	}
	return string(all), nil
}
