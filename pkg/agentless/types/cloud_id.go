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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
)

// CloudProvider represents a cloud provider.
type CloudProvider string

const (
	// CloudProviderNone represents a non-cloud provider.
	CloudProviderNone CloudProvider = "none"
	// CloudProviderAWS represents the Amazon Web Services cloud provider.
	CloudProviderAWS CloudProvider = "aws"
	// CloudProviderAzure represents the Microsoft Azure cloud provider.
	CloudProviderAzure CloudProvider = "azure"
)

// CloudID represents an Cloud Resource Identifier.
// ie. an ARN for Amazon resources.
type CloudID struct {
	// Mandatory fields
	provider     CloudProvider
	region       string
	accountID    string
	resource     string
	resourceType ResourceType
	resourceName string

	// CloudProviderNone
	path string

	// CloudProviderAWS
	arn string

	// CloudProviderAzure
	resourceID string
}

// AsText returns the string representation of the CloudID.
func (id CloudID) AsText() string {
	switch id.provider {
	case CloudProviderNone:
		return fmt.Sprintf("localhost:%s", id.path)
	case CloudProviderAWS:
		return id.arn
	case CloudProviderAzure:
		return id.resourceID
	}
	panic(fmt.Errorf("unimplemented: %#+v", id))
}

// AsAzureID converts an Azure CloudID to an arm.ResourceID.
func (id CloudID) AsAzureID() (*arm.ResourceID, error) {
	if id.provider != CloudProviderAzure {
		return nil, fmt.Errorf("AsAzureID() is only supported for Azure resources")
	}
	return arm.ParseResourceID(id.resourceID)
}

// Provider returns the cloud provider of the resource.
func (id CloudID) Provider() CloudProvider {
	return id.provider
}

// Region return the region of the resource.
func (id CloudID) Region() string {
	return id.region
}

// AccountID return the account ID of the resource.
func (id CloudID) AccountID() string {
	return id.accountID
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
	return id.AsText()
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
	return []byte(id.AsText()), nil
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
			return CloudID{}, fmt.Errorf("bad cloud id: invalid number of sections: %q", s)
		}
		p := filepath.Join("/", sections[1])
		id = CloudID{
			provider:     CloudProviderNone,
			region:       "localhost",
			accountID:    "localhost",
			resource:     p,
			resourceType: ResourceTypeLocalDir,
			resourceName: p,
		}
	} else if strings.HasPrefix(s, "arn:") {
		id, err = parseAWSARN(s)
		if err != nil {
			return CloudID{}, err
		}
	} else if strings.HasPrefix(s, "azure:") {
		id, err = ParseAzureResourceID(s[len("azure:"):])
		if err != nil {
			return CloudID{}, err
		}
	} else {
		return CloudID{}, fmt.Errorf("bad cloud id: invalid prefix: %q", s)
	}
	isExpected := len(expectedTypes) == 0
	for _, t := range expectedTypes {
		if t == id.resourceType {
			isExpected = true
			break
		}
	}
	if !isExpected {
		return CloudID{}, fmt.Errorf("bad cloud id: expecting one of these resource types %v but got %s", expectedTypes, id.resourceType)
	}
	return id, nil
}

// AWSCloudID returns an CloudID for the given AWS resource.
func AWSCloudID(region, accountID string, resourceType ResourceType, resourceName string) (CloudID, error) {
	var resource, service string
	switch resourceType {
	case ResourceTypeVolume:
		service = "ec2"
		resource = fmt.Sprintf("volume/%s", resourceName)
	case ResourceTypeSnapshot:
		service = "ec2"
		resource = fmt.Sprintf("snapshot/%s", resourceName)
	case ResourceTypeHostImage:
		service = "ec2"
		resource = fmt.Sprintf("image/%s", resourceName)
	case ResourceTypeFunction:
		service = "lambda"
		resource = fmt.Sprintf("function:%s", resourceName)
	case ResourceTypeRole:
		service = "iam"
		resource = fmt.Sprintf("role/%s", resourceName)
	default:
		return CloudID{}, fmt.Errorf("unsupported resource type for AWS: %s", resourceType)
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

func parseAWSARN(s string) (CloudID, error) {
	sections := strings.SplitN(s, ":", 6)
	if len(sections) != 6 {
		return CloudID{}, fmt.Errorf("bad cloud id: invalid number of sections: %q", s)
	}
	if sections[0] != "arn" {
		return CloudID{}, fmt.Errorf("bad cloud id: unexpected prefix %q for %q", sections[0], s)
	}
	partition := sections[1]
	service := sections[2]
	region := sections[3]
	accountID := sections[4]
	resource := sections[5]
	if !partitionReg.MatchString(partition) {
		return CloudID{}, fmt.Errorf("bad cloud id %q: unexpected partition", s)
	}
	if region != "" && !regionReg.MatchString(region) {
		return CloudID{}, fmt.Errorf("bad cloud id %q: unexpected region (should be empty or match %s)", s, regionReg)
	}
	if accountID != "" && !accountIDReg.MatchString(accountID) {
		return CloudID{}, fmt.Errorf("bad cloud id %q: unexpected account ID (should match %s)", s, accountIDReg)
	}
	var resourceType ResourceType
	var resourceName string
	switch {
	case service == "ec2" && strings.HasPrefix(resource, "volume/"):
		resourceType, resourceName = ResourceTypeVolume, strings.TrimPrefix(resource, "volume/")
		if !strings.HasPrefix(resourceName, "vol-") {
			return CloudID{}, fmt.Errorf("bad cloud id %q: resource ID has wrong prefix", s)
		}
		if !resourceNameReg.MatchString(strings.TrimPrefix(resourceName, "vol-")) {
			return CloudID{}, fmt.Errorf("bad cloud id %q: resource ID has wrong format (should match %s)", s, resourceNameReg)
		}
	case service == "ec2" && strings.HasPrefix(resource, "snapshot/"):
		resourceType, resourceName = ResourceTypeSnapshot, strings.TrimPrefix(resource, "snapshot/")
		if !strings.HasPrefix(resourceName, "snap-") {
			return CloudID{}, fmt.Errorf("bad cloud id %q: resource ID has wrong prefix", s)
		}
		if !resourceNameReg.MatchString(strings.TrimPrefix(resourceName, "snap-")) {
			return CloudID{}, fmt.Errorf("bad cloud id %q: resource ID has wrong format (should match %s)", s, resourceNameReg)
		}
	case service == "ec2" && strings.HasPrefix(resource, "image/"):
		resourceType, resourceName = ResourceTypeHostImage, strings.TrimPrefix(resource, "image/")
		if !strings.HasPrefix(resourceName, "ami-") {
			return CloudID{}, fmt.Errorf("bad cloud id %q: resource ID has wrong prefix", s)
		}
		if !resourceNameReg.MatchString(strings.TrimPrefix(resourceName, "ami-")) {
			return CloudID{}, fmt.Errorf("bad cloud id %q: resource ID has wrong format (should match %s)", s, resourceNameReg)
		}
	case service == "lambda" && strings.HasPrefix(resource, "function:"):
		resourceType, resourceName = ResourceTypeFunction, strings.TrimPrefix(resource, "function:")
		if sep := strings.Index(resourceName, ":"); sep > 0 {
			resourceName = resourceName[:sep]
		}
		if !functionReg.MatchString(resourceName) {
			return CloudID{}, fmt.Errorf("bad cloud id %q: function name has wrong format (should match %s)", s, functionReg)
		}
	case service == "sts" && strings.HasPrefix(resource, "assumed-role/"):
		resourceType, resourceName = ResourceTypeRole, strings.TrimPrefix(resource, "assumed-role/")
		resourceName = strings.SplitN(resourceName, "/", 2)[0]
		if !roleNameReg.MatchString(resourceName) {
			return CloudID{}, fmt.Errorf("bad cloud id %q: role name has wrong format (should match %s)", s, roleNameReg)
		}
	case service == "iam" && strings.HasPrefix(resource, "role/"):
		resourceType, resourceName = ResourceTypeRole, strings.TrimPrefix(resource, "role/")
		if !roleNameReg.MatchString(resourceName) {
			return CloudID{}, fmt.Errorf("bad cloud id %q: role name has wrong format (should match %s)", s, roleNameReg)
		}
	default:
		return CloudID{}, fmt.Errorf("bad cloud id %q: unexpected resource type", s)
	}
	return CloudID{
		provider:     CloudProviderAWS,
		region:       region,
		accountID:    accountID,
		resource:     resource,
		resourceType: resourceType,
		resourceName: resourceName,

		arn: s,
	}, nil
}

// FromAzureResourceID returns a CloudID for the given Azure resource.
func FromAzureResourceID(resourceID *arm.ResourceID) CloudID {
	var resourceType ResourceType
	switch resourceID.ResourceType.String() {
	case "Microsoft.Compute/snapshots":
		resourceType = ResourceTypeSnapshot
	case "Microsoft.Compute/disks":
		resourceType = ResourceTypeVolume
	case "Microsoft.ManagedIdentity/userAssignedIdentities":
		resourceType = ResourceTypeRole
	}

	return CloudID{
		provider: CloudProviderAzure,
		//region: string
		accountID:    resourceID.SubscriptionID,
		resource:     resourceID.ResourceType.String() + "/" + resourceID.Name,
		resourceType: resourceType,
		resourceName: resourceID.Name,
		resourceID:   resourceID.String(),
	}
}

// ParseAzureResourceID parses an Azure resource identifier.
func ParseAzureResourceID(s string) (CloudID, error) {
	resourceID, err := arm.ParseResourceID(s)
	if err != nil {
		return CloudID{}, err
	}
	return FromAzureResourceID(resourceID), nil
}

// HumanParseCloudID parses an Cloud Identifier string or a resource
// identifier and returns an cloud identifier. Helpful for CLI interface.
func HumanParseCloudID(s string, provider CloudProvider, region, accountID string, expectedTypes ...ResourceType) (CloudID, error) {
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
		} else if strings.HasPrefix(s, "ami-") {
			service = "ec2"
			s = "image/" + s
		} else if strings.HasPrefix(s, "function:") {
			service = "lambda"
		}
		if service != "" {
			arn := fmt.Sprintf("arn:aws:%s:%s:%s:%s", service, region, accountID, s)
			return ParseCloudID(arn, expectedTypes...)
		}
	}

	// Azure
	if provider == CloudProviderAzure {
		return ParseCloudID(fmt.Sprintf("azure:%s", s), expectedTypes...)
	}

	// Localhost
	if strings.HasPrefix(s, "/") && (len(s) == 1 || fs.ValidPath(s[1:])) {
		return ParseCloudID(fmt.Sprintf("localhost:%s", s), expectedTypes...)
	}

	return CloudID{}, fmt.Errorf("unable to parse resource %q", s)
}
