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

	// Mock metadata values
	getInstanceID = func(ctx context.Context) (string, error) {
		return "i-abcdef1234567890", nil
	}
	getRegion = func(ctx context.Context) (string, error) {
		return "us-west-2", nil
	}
	getAccountID = func(ctx context.Context) (string, error) {
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
