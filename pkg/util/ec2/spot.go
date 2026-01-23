// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cachedfetch"
	ec2internal "github.com/DataDog/datadog-agent/pkg/util/ec2/internal"
)

const (
	imdsSpotInstanceAction = "/spot/instance-action"
	imdsInstanceLifeCycle  = "/instance-life-cycle"
	instanceLifeCycleSpot  = "spot"
)

// ErrNotSpotInstance is returned when the instance is not a spot instance
var ErrNotSpotInstance = errors.New("instance is not a spot instance")

// spotInstanceAction represents the response from the spot/instance-action IMDS endpoint
type spotInstanceAction struct {
	Action string `json:"action"`
	Time   string `json:"time"`
}

var instanceLifeCycleFetcher = cachedfetch.Fetcher{
	Name: "EC2 Instance Life Cycle",
	Attempt: func(ctx context.Context) (interface{}, error) {
		return ec2internal.GetMetadataItem(ctx, imdsInstanceLifeCycle, ec2internal.UseIMDSv2(), false)
	},
}

// IsSpotInstance returns true if the current EC2 instance is a spot instance
func IsSpotInstance(ctx context.Context) (bool, error) {
	lifecycle, err := instanceLifeCycleFetcher.FetchString(ctx)
	if err != nil {
		return false, fmt.Errorf("unable to retrieve instance-life-cycle from IMDS: %w", err)
	}
	return lifecycle == instanceLifeCycleSpot, nil
}

// GetSpotTerminationTime returns the scheduled termination time for a spot instance.
// If the instance is not a spot instance, it returns ErrNotSpotInstance.
// If no termination is scheduled, it returns an error (typically a 404 from IMDS).
// Docs: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/spot-instance-termination-notices.html
func GetSpotTerminationTime(ctx context.Context) (time.Time, error) {
	// First check if this is a spot instance
	isSpot, err := IsSpotInstance(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("unable to determine instance type: %w", err)
	}
	if !isSpot {
		return time.Time{}, ErrNotSpotInstance
	}

	res, err := ec2internal.GetMetadataItem(ctx, imdsSpotInstanceAction, ec2internal.UseIMDSv2(), false)
	if err != nil {
		return time.Time{}, fmt.Errorf("unable to retrieve spot instance-action from IMDS: %w", err)
	}

	var action spotInstanceAction
	if err := json.Unmarshal([]byte(res), &action); err != nil {
		return time.Time{}, fmt.Errorf("unable to parse spot instance-action response: %w", err)
	}

	terminationTime, err := time.Parse(time.RFC3339, action.Time)
	if err != nil {
		return time.Time{}, fmt.Errorf("unable to parse termination time %q: %w", action.Time, err)
	}

	return terminationTime, nil
}
