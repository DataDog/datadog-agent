// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build ec2

package ec2

import (
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

func fetchEc2Tags() ([]string, error) {
	instanceIdentity, err := getInstanceIdentity()
	if err != nil {
		return nil, err
	}

	iamParams, err := getSecurityCreds()
	if err != nil {
		return nil, err
	}

	awsCreds := credentials.NewStaticCredentials(iamParams.AccessKeyID,
		iamParams.SecretAccessKey,
		iamParams.Token)

	awsSess, err := session.NewSession(&aws.Config{
		Region:      aws.String(instanceIdentity.Region),
		Credentials: awsCreds,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to get aws session, %s", err)
	}

	connection := ec2.New(awsSess)
	ec2Tags, err := connection.DescribeTags(&ec2.DescribeTagsInput{
		Filters: []*ec2.Filter{{
			Name: aws.String("resource-id"),
			Values: []*string{
				aws.String(instanceIdentity.InstanceID),
			},
		}},
	})

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

// GetTags grabs the host tags from the EC2 api
func GetTags() ([]string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return nil, fmt.Errorf("cloud provider is disabled by configuration")
	}

	tags, err := fetchTags()
	if err != nil {
		if ec2Tags, found := cache.Cache.Get(tagsCacheKey); found {
			log.Infof("unable to get tags from aws, returning cached tags: %s", err)
			return ec2Tags.([]string), nil
		}
		return nil, log.Warnf("unable to get tags from aws and cache is empty: %s", err)
	}

	// save tags to the cache in case we exceed quotas later
	cache.Cache.Set(tagsCacheKey, tags, cache.NoExpiration)

	return tags, nil
}

type ec2Identity struct {
	Region     string
	InstanceID string
}

func getInstanceIdentity() (*ec2Identity, error) {
	instanceIdentity := &ec2Identity{}

	res, err := doHTTPRequest(instanceIdentityURL, http.MethodGet, map[string]string{}, true)
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

func getSecurityCreds() (*ec2SecurityCred, error) {
	iamParams := &ec2SecurityCred{}

	iamRole, err := getIAMRole()
	if err != nil {
		return iamParams, err
	}

	res, err := doHTTPRequest(metadataURL+"/iam/security-credentials/"+iamRole, http.MethodGet, map[string]string{}, true)
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

func getIAMRole() (string, error) {
	res, err := doHTTPRequest(metadataURL+"/iam/security-credentials/", http.MethodGet, map[string]string{}, true)
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
