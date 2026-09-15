// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// This is the Go equivalent of the Python `setup_aws_sso_config`
// (tasks/e2e_framework/setup/aws.py). It lets the demo CLI provision AWS
// resources without an outer `aws-vault exec` wrapper, exactly like
// `dda inv create-vm`.
//
// The trick is an AWS `credential_process` profile: we write an
// `exec-sso-<account>-<role>` profile whose credentials are produced on demand
// by `aws-vault exec <plain-profile> --json`. Pulumi's AWS provider is pointed
// at that profile (env.Profile() defaults to it for the agent-sandbox
// environment), so the AWS SDK invokes aws-vault itself when it needs
// credentials -- no wrapper required.
const (
	// awsSSOAccountID is the AWS SSO account the E2E framework authenticates
	// against. Mirrors `acct_id` in tasks/e2e_framework/setup/aws.py.
	awsSSOAccountID = "376334461865"
	// awsSSOStartURL is the SSO portal start URL.
	awsSSOStartURL = "https://d-906757b57c.awsapps.com/start/#"
	// awsSSORegion is the region every framework resource is pinned to.
	awsSSORegion = "us-east-1"

	awsSSOConfigBeginMarker = "# BEGIN Automatically added by agent-health-demo"
	awsSSOConfigEndMarker   = "# END Automatically added by agent-health-demo"
)

// accountAdminRoleByAccount maps an account to its admin role. Accounts not
// listed default to "account-admin". Keep in sync with the `profile:` entries
// in test/e2e-framework/resources/aws/environmentDefaults.go and with
// ACCOUNT_ADMIN_ROLE_BY_ACCOUNT in tasks/e2e_framework/setup/aws.py.
var accountAdminRoleByAccount = map[string]string{
	"agent-sandbox": "account-admin-8h",
}

// accountFromInfraEnv extracts the account name from an infra environment such
// as "aws/agent-sandbox".
func accountFromInfraEnv(infraEnv string) string {
	if i := strings.LastIndex(infraEnv, "/"); i >= 0 {
		return infraEnv[i+1:]
	}
	return infraEnv
}

// ensureAWSSSOConfig appends the SSO profiles for the given infra environment to
// ~/.aws/config when they are missing, and returns the credential_process
// profile name Pulumi should use. It is idempotent: if the profile is already
// present the file is left untouched.
//
// It mirrors setup_aws_sso_config(interactive=False): the profiles are added
// unconditionally, without prompting.
func ensureAWSSSOConfig(infraEnv string) (execProfile string, err error) {
	account := accountFromInfraEnv(infraEnv)
	role := accountAdminRoleByAccount[account]
	if role == "" {
		role = "account-admin"
	}
	profileName := fmt.Sprintf("sso-%s-%s", account, role)
	execProfile = fmt.Sprintf("exec-%s", profileName)

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	confPath := filepath.Join(home, ".aws", "config")

	// Skip if the profile is already present, matching the Python's substring check.
	if data, err := os.ReadFile(confPath); err == nil {
		if strings.Contains(string(data), fmt.Sprintf("[profile %s]", execProfile)) {
			return execProfile, nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	block := fmt.Sprintf(`
%s

[profile %s]
sso_session = %s
sso_account_id = %s
sso_role_name = %s
region = %s
sso_region = %s
sso_start_url = %s

[profile %s]
credential_process = aws-vault exec %s --json

[sso-session %s]
sso_region = %s
sso_start_url = %s
sso_registration_scopes = sso:account:access

%s
`,
		awsSSOConfigBeginMarker,
		profileName, profileName, awsSSOAccountID, role, awsSSORegion, awsSSORegion, awsSSOStartURL,
		execProfile, profileName,
		profileName, awsSSORegion, awsSSOStartURL,
		awsSSOConfigEndMarker,
	)

	if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(confPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(block); err != nil {
		return "", err
	}
	fmt.Printf("Wrote AWS SSO profile %q to %s\n", execProfile, confPath)
	return execProfile, nil
}

// prepareAWSCredentials makes AWS credentials available to Pulumi's AWS provider
// without an outer `aws-vault exec` wrapper. It ensures the credential_process
// profile exists in ~/.aws/config and, unless the caller already provides static
// credentials, points the AWS SDK at that profile via AWS_PROFILE so it invokes
// aws-vault on demand.
func prepareAWSCredentials(infraEnv string) error {
	execProfile, err := ensureAWSSSOConfig(infraEnv)
	if err != nil {
		return fmt.Errorf("failed to set up AWS SSO profile: %w", err)
	}
	// Static credentials (e.g. an active `aws-vault exec` session) take precedence;
	// leave them alone. Otherwise select the credential_process profile so the SDK
	// runs aws-vault itself.
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
		return nil
	}
	if os.Getenv("AWS_PROFILE") == "" {
		if err := os.Setenv("AWS_PROFILE", execProfile); err != nil {
			return err
		}
	}
	return nil
}
