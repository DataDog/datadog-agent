// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"testing"
)

func TestGetHostCCRID(t *testing.T) {
	origGetInstanceID := getInstanceID
	origGetRegion := getRegion
	origGetAccountID := getAccountID

	defer func() {
		getInstanceID = origGetInstanceID
		getRegion = origGetRegion
		getAccountID = origGetAccountID
	}()

	getInstanceID = func(_ context.Context) (string, error) {
		return "i-abcdef1234567890", nil
	}
	getRegion = func(_ context.Context) (string, error) {
		return "us-west-2", nil
	}
	getAccountID = func(_ context.Context) (string, error) {
		return "123456789012", nil
	}

	ctx := context.Background()
	arn, err := GetHostCCRID(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "arn:aws:ec2:us-west-2:123456789012:instance/i-abcdef1234567890"
	if arn != expected {
		t.Errorf("expected ARN %q, got %q", expected, arn)
	}
}
