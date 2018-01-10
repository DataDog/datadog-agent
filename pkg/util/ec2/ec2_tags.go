// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build ec2

package ec2

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
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

	awsCreds := credentials.NewStaticCredentials(iamParams["AccessKeyId"], iamParams["SecretAccessKey"], iamParams["Token"])

	awsSess, err := session.NewSession(&aws.Config{
		Region:      aws.String(instanceIdentity["region"]),
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
				aws.String(instanceIdentity["instanceId"]),
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

func getInstanceIdentity() (map[string]string, error) {
	instanceIdentity := map[string]string{}

	res, err := getResponse(instanceIdentityURL)
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

func getSecurityCreds() (map[string]string, error) {
	iamParams := map[string]string{}

	iamRole, err := getIAMRole()
	if err != nil {
		return iamParams, err
	}

	res, err := getResponse(metadataURL + "/iam/security-credentials/" + iamRole + "/")
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
	res, err := getResponse(metadataURL + "/iam/security-credentials/")
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
