// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package creds

import (
	"context"
	"fmt"
	"os"

	ec2internal "github.com/DataDog/datadog-agent/pkg/util/aws/creds/internal"
)

// HasAWSCredentialsInEnvironment checks if AWS credentials are available in environment variables.
// This checks for AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY, which are the standard AWS SDK env vars.
func HasAWSCredentialsInEnvironment() bool {
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	return accessKeyID != "" && secretAccessKey != ""
}

// HasAWSWorkloadIdentityInEnvironment returns true if IRSA (EKS web identity) env vars are present.
func HasAWSWorkloadIdentityInEnvironment() bool {
	return os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") != "" && os.Getenv("AWS_ROLE_ARN") != ""
}

// HasAWSContainerCredentialsInEnvironment returns true if ECS/EKS Pod Identity container credential env vars are present.
func HasAWSContainerCredentialsInEnvironment() bool {
	return os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" ||
		os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI") != ""
}

// IncompleteAWSCredentialEnv describes a credential mechanism whose environment variables are only
// half set, or returns "" when none is. The static and IRSA predicates above require both variables
// of their pair, so a half-configured mechanism is skipped and a lower-precedence one is used
// instead. That fall-through is deliberate: erroring would turn an unrelated stray variable in the
// environment into an authentication outage. But it should not be silent, because the operator ends
// up authenticating as a different principal than they configured, and nothing else in the logs
// would say so.
//
// AWS_CONTAINER_CREDENTIALS_RELATIVE_URI / _FULL_URI are not checked: either alone is a complete
// configuration, which is why their predicate uses OR.
func IncompleteAWSCredentialEnv() string {
	set := func(k string) bool { return os.Getenv(k) != "" }
	switch {
	case set("AWS_ACCESS_KEY_ID") && !set("AWS_SECRET_ACCESS_KEY"):
		return "AWS_ACCESS_KEY_ID is set but AWS_SECRET_ACCESS_KEY is not"
	case set("AWS_SECRET_ACCESS_KEY") && !set("AWS_ACCESS_KEY_ID"):
		return "AWS_SECRET_ACCESS_KEY is set but AWS_ACCESS_KEY_ID is not"
	case set("AWS_ROLE_ARN") && !set("AWS_WEB_IDENTITY_TOKEN_FILE"):
		return "AWS_ROLE_ARN is set but AWS_WEB_IDENTITY_TOKEN_FILE is not"
	case set("AWS_WEB_IDENTITY_TOKEN_FILE") && !set("AWS_ROLE_ARN"):
		return "AWS_WEB_IDENTITY_TOKEN_FILE is set but AWS_ROLE_ARN is not"
	}
	return ""
}

// Credential source names reported by DetectAWSCredentialSource. They describe the mechanism that
// will supply credentials, not the provider implementation that resolves them.
const (
	// SourceEnvironment means static AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY are set.
	SourceEnvironment = "static environment variables"
	// SourceWebIdentity means IRSA / EKS web-identity env vars are set.
	SourceWebIdentity = "IRSA web identity"
	// SourceContainer means ECS task-role / EKS Pod Identity container credential env vars are set.
	SourceContainer = "ECS/EKS container credentials"
	// SourceIMDS means no env-based source was present but IMDS answered.
	SourceIMDS = "EC2 IMDS"
)

// DetectAWSCredentialSource reports which AWS credential mechanism this workload can use, checking
// static credentials, IRSA, container credentials, and IMDS in that order (the same precedence
// resolveCredentials uses to pick a provider). It returns the matched source name, or an error
// describing why no source was found so callers can log something actionable: the env-based checks
// are pure environment inspection, so a failure here means none of the expected variables were set
// and the IMDS fallback also did not answer.
func DetectAWSCredentialSource(ctx context.Context) (string, error) {
	// Static credentials in environment
	if HasAWSCredentialsInEnvironment() {
		return SourceEnvironment, nil
	}
	// IRSA / EKS web identity
	if HasAWSWorkloadIdentityInEnvironment() {
		return SourceWebIdentity, nil
	}
	// ECS task role or EKS Pod Identity container credentials
	if HasAWSContainerCredentialsInEnvironment() {
		return SourceContainer, nil
	}

	// Only the IMDS leg contacts the network, and it is the one an operator can forbid: honor
	// cloud_provider_metadata before probing. GetInstanceIdentity reaches DoHTTPRequest directly
	// rather than going through GetMetadataItem, so the check does not happen for us.
	if err := ec2internal.CheckCloudProviderEnabled(); err != nil {
		return "", fmt.Errorf("no AWS credential source found: AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY, "+
			"AWS_ROLE_ARN+AWS_WEB_IDENTITY_TOKEN_FILE (IRSA) and "+
			"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI/_FULL_URI (ECS/EKS Pod Identity) are all unset, "+
			"and the EC2 IMDS fallback is not permitted: %w", err)
	}

	// Try to fetch instance identity document using ImdsAllVersions
	// This will try IMDSv2 first, then fallback to IMDSv1
	if _, err := ec2internal.GetInstanceIdentity(ctx); err != nil {
		return "", fmt.Errorf("no AWS credential source found: AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY, "+
			"AWS_ROLE_ARN+AWS_WEB_IDENTITY_TOKEN_FILE (IRSA) and "+
			"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI/_FULL_URI (ECS/EKS Pod Identity) are all unset, "+
			"and EC2 IMDS did not answer: %w", err)
	}
	return SourceIMDS, nil
}

// GetAWSRegion returns the AWS region for the current EC2 instance or from environment.
// Returns empty string and error if not running on AWS or if region cannot be determined.
// This function tries multiple methods in order:
// 1. AWS_REGION or AWS_DEFAULT_REGION environment variables
// 2. IMDS instance identity document (tries IMDSv2 first, then IMDSv1)
func GetAWSRegion(ctx context.Context) (string, error) {
	// First check environment variables (standard AWS SDK behavior)
	if region := os.Getenv("AWS_REGION"); region != "" {
		return region, nil
	}
	if region := os.Getenv("AWS_DEFAULT_REGION"); region != "" {
		return region, nil
	}

	// Only this leg contacts the network, so honor cloud_provider_metadata before probing, the same
	// as DetectAWSCredentialSource and GetSecurityCredentials. Returning an error costs the caller
	// nothing: it treats a region failure as "use the default region", which is also what an
	// IRSA-only pod with no AWS_REGION already gets.
	if err := ec2internal.CheckCloudProviderEnabled(); err != nil {
		return "", err
	}

	// Try to get region from IMDS (uses ImdsAllVersions to try v2, then v1)
	identity, err := ec2internal.GetInstanceIdentity(ctx)
	if err != nil {
		return "", err
	}

	return identity.Region, nil
}
