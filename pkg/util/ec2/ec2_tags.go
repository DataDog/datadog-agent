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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// declare these as vars not const to ease testing
var (
	instanceIdentityURL = "http://169.254.169.254/latest/dynamic/instance-identity/document/"
)

// GetTags grabs the host tags from the EC2 api
func GetTags() ([]string, error) {
	tags := []string{}

	instanceIdentity, err := getInstanceIdentity()
	if err != nil {
		return tags, err
	}

	iamParams, err := getSecurityCreds()
	if err != nil {
		return tags, err
	}

	awsCreds := credentials.NewStaticCredentials(iamParams.AccessKeyID,
		iamParams.SecretAccessKey,
		iamParams.Token)

	awsSess, err := session.NewSession(&aws.Config{
		Region:      aws.String(instanceIdentity.Region),
		Credentials: awsCreds,
	})
	if err != nil {
		return tags, fmt.Errorf("unable to get aws session, %s", err)
	}

	connection := ec2.New(awsSess)
	grabbedTags, err := connection.DescribeTags(&ec2.DescribeTagsInput{
		Filters: []*ec2.Filter{{
			Name: aws.String("resource-id"),
			Values: []*string{
				aws.String(instanceIdentity.InstanceID),
			},
		}},
	})
	if err != nil {
		return tags, fmt.Errorf("unable to get tags from aws, %s", err)
	}

	for _, tag := range grabbedTags.Tags {
		tags = append(tags, fmt.Sprintf("%s:%s", *tag.Key, *tag.Value))
	}

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
