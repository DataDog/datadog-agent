package types

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// ARN represents an Amazon Resource Name.
type ARN struct {
	Partition    string
	Service      string
	Region       string
	AccountID    string
	ResourceType ResourceType
	ResourceName string

	resource string
}

func (a ARN) String() string {
	return fmt.Sprintf("arn:%s:%s:%s:%s:%s", a.Partition, a.Service, a.Region, a.AccountID, a.resource)
}

// ParseARN parses an ARN and checks that it is of the expected type.
func ParseARN(s string, expectedTypes ...ResourceType) (ARN, error) {
	if !strings.HasPrefix(s, "arn:") {
		return ARN{}, errors.New("arn: invalid prefix")
	}
	sections := strings.SplitN(s, ":", 6)
	if len(sections) != 6 {
		return ARN{}, errors.New("arn: invalid number of sections")
	}
	a := ARN{
		Partition: sections[1],
		Service:   sections[2],
		Region:    sections[3],
		AccountID: sections[4],
		resource:  sections[5],
	}
	var err error
	a.ResourceType, a.ResourceName, err = getARNResource(a)
	if err != nil {
		return ARN{}, err
	}
	isExpected := len(expectedTypes) == 0
	for _, t := range expectedTypes {
		if t == a.ResourceType {
			isExpected = true
			break
		}
	}
	if !isExpected {
		return ARN{}, fmt.Errorf("bad arn: expecting one of these resource types %v but got %s", expectedTypes, a.ResourceType)
	}
	return a, nil
}

var (
	partitionReg    = regexp.MustCompile("^aws[a-zA-Z-]*$")
	regionReg       = regexp.MustCompile("^[a-z]{2}((-gov)|(-iso(b?)))?-[a-z]+-[0-9]{1}$")
	accountIDReg    = regexp.MustCompile("^[0-9]{12}$")
	resourceNameReg = regexp.MustCompile("^[a-f0-9]+$")
	roleNameReg     = regexp.MustCompile("^[a-zA-Z0-9_+=,.@-]{1,64}$")
	functionReg     = regexp.MustCompile(`^([a-zA-Z0-9-_.]+)(:(\$LATEST|[a-zA-Z0-9-_]+))?$`)
)

func getARNResource(arn ARN) (resourceType ResourceType, resourceName string, err error) {
	if arn.Partition == "localhost" {
		return ResourceTypeLocalDir, filepath.Join("/", arn.resource), nil
	}
	if !partitionReg.MatchString(arn.Partition) {
		err = fmt.Errorf("bad arn %q: unexpected partition", arn)
		return
	}
	if arn.Region != "" && !regionReg.MatchString(arn.Region) {
		err = fmt.Errorf("bad arn %q: unexpected region (should be empty or match %s)", arn, regionReg)
		return
	}
	if arn.AccountID != "" && !accountIDReg.MatchString(arn.AccountID) {
		err = fmt.Errorf("bad arn %q: unexpected account ID (should match %s)", arn, accountIDReg)
		return
	}
	switch {
	case arn.Service == "ec2" && strings.HasPrefix(arn.resource, "volume/"):
		resourceType, resourceName = ResourceTypeVolume, strings.TrimPrefix(arn.resource, "volume/")
		if !strings.HasPrefix(resourceName, "vol-") {
			err = fmt.Errorf("bad arn %q: resource ID has wrong prefix", arn)
			return
		}
		if !resourceNameReg.MatchString(strings.TrimPrefix(resourceName, "vol-")) {
			err = fmt.Errorf("bad arn %q: resource ID has wrong format (should match %s)", arn, resourceNameReg)
			return
		}
	case arn.Service == "ec2" && strings.HasPrefix(arn.resource, "snapshot/"):
		resourceType, resourceName = ResourceTypeSnapshot, strings.TrimPrefix(arn.resource, "snapshot/")
		if !strings.HasPrefix(resourceName, "snap-") {
			err = fmt.Errorf("bad arn %q: resource ID has wrong prefix", arn)
			return
		}
		if !resourceNameReg.MatchString(strings.TrimPrefix(resourceName, "snap-")) {
			err = fmt.Errorf("bad arn %q: resource ID has wrong format (should match %s)", arn, resourceNameReg)
			return
		}
	case arn.Service == "lambda" && strings.HasPrefix(arn.resource, "function:"):
		resourceType, resourceName = ResourceTypeFunction, strings.TrimPrefix(arn.resource, "function:")
		if sep := strings.Index(resourceName, ":"); sep > 0 {
			resourceName = resourceName[:sep]
		}
		if !functionReg.MatchString(resourceName) {
			err = fmt.Errorf("bad arn %q: function name has wrong format (should match %s)", arn, functionReg)
		}
	case arn.Service == "sts" && strings.HasPrefix(arn.resource, "assumed-role/"):
		resourceType, resourceName = ResourceTypeRole, strings.TrimPrefix(arn.resource, "assumed-role/")
		if !roleNameReg.MatchString(resourceName) {
			err = fmt.Errorf("bad arn %q: role name has wrong format (should match %s)", arn, roleNameReg)
			return
		}
	case arn.Service == "iam" && strings.HasPrefix(arn.resource, "role/"):
		resourceType, resourceName = ResourceTypeRole, strings.TrimPrefix(arn.resource, "role/")
		if !roleNameReg.MatchString(resourceName) {
			err = fmt.Errorf("bad arn %q: role name has wrong format (should match %s)", arn, roleNameReg)
			return
		}
	default:
		err = fmt.Errorf("bad arn %q: unexpected resource type", arn)
		return
	}
	return
}
