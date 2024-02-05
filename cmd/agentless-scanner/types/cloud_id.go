package types

import (
	"encoding"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// CloudID represents an Cloud Resource Identifier.
// ie. an ARN for Amazon resources.
type CloudID struct {
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
	return fmt.Sprintf("arn:%s:%s:%s:%s:%s", id.Partition, id.Service, id.Region, id.AccountID, id.Resource)
}

var (
	_ encoding.TextMarshaler   = CloudID{}
	_ encoding.TextUnmarshaler = &CloudID{}
)

// ParseCloudID parses an cloud resource identifier and checks that it is of
// the expected type.
func ParseCloudID(s string, expectedTypes ...ResourceType) (CloudID, error) {
	if !strings.HasPrefix(s, "arn:") {
		return CloudID{}, errors.New("cloudID: invalid prefix")
	}
	sections := strings.SplitN(s, ":", 6)
	if len(sections) != 6 {
		return CloudID{}, errors.New("cloudID: invalid number of sections")
	}
	id := CloudID{
		Partition: sections[1],
		Service:   sections[2],
		Region:    sections[3],
		AccountID: sections[4],
		Resource:  sections[5],
	}
	var err error
	id.resourceType, id.resourceName, err = parseAWSCloudID(id)
	if err != nil {
		return CloudID{}, err
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
	if id.Partition == "localhost" {
		return ResourceTypeLocalDir, filepath.Join("/", resource), nil
	}
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
