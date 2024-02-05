// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package types

import (
	"encoding"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

// CloudProvider represents a cloud provider.
type CloudProvider string

const (
	// CloudProviderNone represents a non-cloud provider.
	CloudProviderNone CloudProvider = "none"
	// CloudProviderAWS represents the Amazon Web Services cloud provider.
	CloudProviderAWS CloudProvider = "aws"
)

// CloudID represents an Cloud Resource Identifier.
// ie. an ARN for Amazon resources.
type CloudID struct {
	Provider  CloudProvider
	Partition string
	Service   string
	Region    string
	AccountID string
	Resource  string

	resourceType ResourceType
	resourceName string
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (id *CloudID) UnmarshalText(text []byte) error {
	v, err := ParseCloudID(string(text))
	if err != nil {
		return err
	}
	*id = v
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (id CloudID) MarshalText() (text []byte, err error) {
	return []byte(id.String()), nil
}

// ResourceType returns the type of the resource.
func (id CloudID) ResourceType() ResourceType {
	return id.resourceType
}

// ResourceName returns the name of the resource.
func (id CloudID) ResourceName() string {
	return id.resourceName
}

func (id CloudID) String() string {
	switch id.Provider {
	case CloudProviderNone:
		return fmt.Sprintf("%s:%s", id.Partition, id.Resource)
	case CloudProviderAWS:
		return fmt.Sprintf("arn:%s:%s:%s:%s:%s", id.Partition, id.Service, id.Region, id.AccountID, id.Resource)
	}
	panic("unimplemented")
}

var (
	_ encoding.TextMarshaler   = CloudID{}
	_ encoding.TextUnmarshaler = &CloudID{}
)

// ParseCloudID parses an cloud resource identifier and checks that it is of
// the expected type.
func ParseCloudID(s string, expectedTypes ...ResourceType) (CloudID, error) {
	var err error
	var id CloudID
	if strings.HasPrefix(s, "localhost:") {
		sections := strings.SplitN(s, ":", 2)
		if len(sections) != 2 {
			return CloudID{}, fmt.Errorf("cloudID: invalid number of sections: %q", s)
		}
		p := filepath.Join("/", sections[1])
		id = CloudID{
			Provider:     CloudProviderNone,
			Partition:    sections[0],
			Resource:     p,
			resourceType: ResourceTypeLocalDir,
			resourceName: p,
		}
	} else if strings.HasPrefix(s, "arn:") {
		sections := strings.SplitN(s, ":", 6)
		if len(sections) != 6 {
			return CloudID{}, fmt.Errorf("cloudID: invalid number of sections: %q", s)
		}
		id = CloudID{
			Provider:  CloudProviderAWS,
			Partition: sections[1],
			Service:   sections[2],
			Region:    sections[3],
			AccountID: sections[4],
			Resource:  sections[5],
		}
		id.resourceType, id.resourceName, err = parseAWSCloudID(id)
		if err != nil {
			return CloudID{}, err
		}
	} else {
		return CloudID{}, fmt.Errorf("cloudID: invalid prefix: %q", s)
	}
	isExpected := len(expectedTypes) == 0
	for _, t := range expectedTypes {
		if t == id.resourceType {
			isExpected = true
			break
		}
	}
	if !isExpected {
		return CloudID{}, fmt.Errorf("bad cloudID: expecting one of these resource types %v but got %s", expectedTypes, id.resourceType)
	}
	return id, nil
}

// AWSCloudID returns an CloudID for the given AWS resource.
func AWSCloudID(service, region, accountID string, resourceType ResourceType, resourceName string) (CloudID, error) {
	var resource string
	switch service {
	default:
		resource = fmt.Sprintf("%s/%s", resourceType, resourceName)
	case "lambda":
		resource = fmt.Sprintf("%s:%s", resourceType, resourceName)
	}
	return ParseCloudID(fmt.Sprintf("arn:aws:%s:%s:%s:%s", service, region, accountID, resource))
}

var (
	partitionReg    = regexp.MustCompile("^aws[a-zA-Z-]*$")
	regionReg       = regexp.MustCompile("^[a-z]{2}((-gov)|(-iso(b?)))?-[a-z]+-[0-9]{1}$")
	accountIDReg    = regexp.MustCompile("^[0-9]{12}$")
	resourceNameReg = regexp.MustCompile("^[a-f0-9]+$")
	roleNameReg     = regexp.MustCompile("^[a-zA-Z0-9_+=,.@-]{1,64}$")
	functionReg     = regexp.MustCompile(`^([a-zA-Z0-9-_.]+)(:(\$LATEST|[a-zA-Z0-9-_]+))?$`)
)

func parseAWSCloudID(id CloudID) (resourceType ResourceType, resourceName string, err error) {
	resource := id.Resource
	if !partitionReg.MatchString(id.Partition) {
		err = fmt.Errorf("bad cloud id %q: unexpected partition", id)
		return
	}
	if id.Region != "" && !regionReg.MatchString(id.Region) {
		err = fmt.Errorf("bad cloud id %q: unexpected region (should be empty or match %s)", id, regionReg)
		return
	}
	if id.AccountID != "" && !accountIDReg.MatchString(id.AccountID) {
		err = fmt.Errorf("bad cloud id %q: unexpected account ID (should match %s)", id, accountIDReg)
		return
	}
	switch {
	case id.Service == "ec2" && strings.HasPrefix(resource, "volume/"):
		resourceType, resourceName = ResourceTypeVolume, strings.TrimPrefix(resource, "volume/")
		if !strings.HasPrefix(resourceName, "vol-") {
			err = fmt.Errorf("bad cloud id %q: resource ID has wrong prefix", id)
			return
		}
		if !resourceNameReg.MatchString(strings.TrimPrefix(resourceName, "vol-")) {
			err = fmt.Errorf("bad cloud id %q: resource ID has wrong format (should match %s)", id, resourceNameReg)
			return
		}
	case id.Service == "ec2" && strings.HasPrefix(resource, "snapshot/"):
		resourceType, resourceName = ResourceTypeSnapshot, strings.TrimPrefix(resource, "snapshot/")
		if !strings.HasPrefix(resourceName, "snap-") {
			err = fmt.Errorf("bad cloud id %q: resource ID has wrong prefix", id)
			return
		}
		if !resourceNameReg.MatchString(strings.TrimPrefix(resourceName, "snap-")) {
			err = fmt.Errorf("bad cloud id %q: resource ID has wrong format (should match %s)", id, resourceNameReg)
			return
		}
	case id.Service == "lambda" && strings.HasPrefix(resource, "function:"):
		resourceType, resourceName = ResourceTypeFunction, strings.TrimPrefix(resource, "function:")
		if sep := strings.Index(resourceName, ":"); sep > 0 {
			resourceName = resourceName[:sep]
		}
		if !functionReg.MatchString(resourceName) {
			err = fmt.Errorf("bad cloud id %q: function name has wrong format (should match %s)", id, functionReg)
		}
	case id.Service == "sts" && strings.HasPrefix(resource, "assumed-role/"):
		resourceType, resourceName = ResourceTypeRole, strings.TrimPrefix(resource, "assumed-role/")
		resourceName = strings.SplitN(resourceName, "/", 2)[0]
		if !roleNameReg.MatchString(resourceName) {
			err = fmt.Errorf("bad cloud id %q: role name has wrong format (should match %s)", id, roleNameReg)
			return
		}
	case id.Service == "iam" && strings.HasPrefix(resource, "role/"):
		resourceType, resourceName = ResourceTypeRole, strings.TrimPrefix(resource, "role/")
		if !roleNameReg.MatchString(resourceName) {
			err = fmt.Errorf("bad cloud id %q: role name has wrong format (should match %s)", id, roleNameReg)
			return
		}
	default:
		err = fmt.Errorf("bad cloud id %q: unexpected resource type", id)
		return
	}
	return
}

// HumanParseCloudID parses an Cloud Identifier string or a resource
// identifier and returns an cloud identifier. Helpful for CLI interface.
func HumanParseCloudID(s string, provider CloudProvider, region, accountID string, expectedTypes ...ResourceType) (CloudID, error) {
	// Localhost
	if strings.HasPrefix(s, "/") && (len(s) == 1 || fs.ValidPath(s[1:])) {
		return ParseCloudID(fmt.Sprintf("localhost:%s", s), expectedTypes...)
	}

	// AWS
	if provider == CloudProviderAWS {
		if strings.HasPrefix(s, "arn:") {
			return ParseCloudID(s, expectedTypes...)
		}
		var service string
		if strings.HasPrefix(s, "vol-") {
			service = "ec2"
			s = "volume/" + s
		} else if strings.HasPrefix(s, "snap-") {
			service = "ec2"
			s = "snapshot/" + s
		} else if strings.HasPrefix(s, "function:") {
			service = "lambda"
		}
		if service != "" {
			arn := fmt.Sprintf("arn:aws:%s:%s:%s:%s", service, region, accountID, s)
			return ParseCloudID(arn, expectedTypes...)
		}
	}

	return CloudID{}, fmt.Errorf("unable to parse resource %q", s)
}
