// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package aws provides the implementation for aws auth exchange
package aws

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// signingData is the data structure that represents the Data used to generate an AWS Proof
type signingData struct {
	headersEncoded string
	bodyEncoded    string
	urlEncoded     string
	method         string
}

const (
	// orgIDHeader is the header we use to specify the name of the org we request a token for
	orgIDHeader       = "x-ddog-org-id"
	contentTypeHeader = "Content-Type"
	applicationForm   = "application/x-www-form-urlencoded; charset=utf-8"

	defaultRegion         = "us-east-1"
	defaultStsHost        = "sts.amazonaws.com"
	service               = "sts"
	getCallerIdentityBody = "Action=GetCallerIdentity&Version=2011-06-15"
)

// AWSAuth contains the implementation for the AWS cloud auth
type AWSAuth struct {
	region string

	// lastSource names the credential mechanism selected by the most recent resolveCredentials
	// attempt (ex: "DelegatedAuthIMDS"). It is written before the credentials are fetched, so it
	// reflects what was attempted rather than what succeeded; see CredentialSourceReporter. Read by
	// the status page via common.CredentialSourceReporter. Credentials are resolved on the
	// delegated-auth refresh goroutine and read by whoever renders status, so it is atomic.
	lastSource atomic.Pointer[string]
}

// NewAWSAuth creates a new AWSAuth from an AWSProviderConfig.
func NewAWSAuth(config *cloudauthconfig.AWSProviderConfig) *AWSAuth {
	region := ""
	if config != nil {
		region = config.Region
	}
	return &AWSAuth{
		region: region,
	}
}

// Compile-time check that the status page's optional interface stays satisfied. Without it a rename
// would silently drop the credential source from `agent status` rather than fail the build.
var _ common.CredentialSourceReporter = (*AWSAuth)(nil)

// LastCredentialSource implements common.CredentialSourceReporter. It names the mechanism selected
// for the most recent resolution attempt, whether or not that attempt succeeded, so it is populated
// exactly when an operator needs it most.
func (a *AWSAuth) LastCredentialSource() string {
	if s := a.lastSource.Load(); s != nil {
		return *s
	}
	return ""
}

// GenerateAuthProof generates an AWS-specific authentication proof using SigV4 signing.
// This proof includes a signed AWS STS GetCallerIdentity request that proves access to AWS credentials.
// The context parameter allows for cancellation of the proof generation.
func (a *AWSAuth) GenerateAuthProof(ctx context.Context, cfg pkgconfigmodel.Reader, config *common.AuthConfig) (string, error) {
	// Check for context cancellation early
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	if config == nil || config.OrgUUID == "" {
		return "", errors.New("missing org UUID in config")
	}

	// Get local AWS Credentials. cfg is threaded through so the IRSA web-identity STS call can use
	// the Agent's configured HTTP transport (proxy / custom CA / TLS settings). The error names the
	// credential mechanism that was tried and why it failed, and is returned rather than only logged
	// so it reaches the caller's error log and the status page instead of a generic
	// "missing AWS credentials".
	credentials, err := a.getCredentials(ctx, cfg)
	if err != nil {
		return "", err
	}

	// Use the credentials to generate the signing data
	data, err := a.generateAwsAuthData(ctx, config.OrgUUID, credentials)
	if err != nil {
		return "", err
	}

	// Generate the auth string that will be passed to the Datadog API
	// Format: "<base64-body>|<base64-headers>|<method>|<base64-url>"
	// - body: Base64-encoded request body (GetCallerIdentity action)
	// - headers: Base64-encoded JSON map of HTTP headers (includes Authorization with SigV4 signature)
	// - method: HTTP method (POST)
	// - url: Base64-encoded STS endpoint URL
	authProof := data.bodyEncoded + "|" + data.headersEncoded + "|" + data.method + "|" + data.urlEncoded
	return authProof, nil
}

// getCredentials retrieves AWS credentials from the environment the Agent is running in, using the
// one mechanism that matches it: env, IRSA web identity, ECS/EKS container credentials, or IMDS
// (shared config/SSO/profiles are deliberately unsupported, see resolveCredentials).
//
// A failure here means delegated auth cannot fetch an API key at all, so the error is annotated
// with what the operator should look at for the mechanism that was actually tried, then returned.
// Nothing is logged here: the caller already reports the failure, and logging as well would emit
// the same remediation text three times per attempt.
func (a *AWSAuth) getCredentials(ctx context.Context, cfg pkgconfigmodel.Reader) (*creds.SecurityCredentials, error) {
	resolved, err := a.resolveCredentials(ctx, cfg)
	if err != nil {
		// A canceled context means the Agent is shutting down or this instance was replaced, not that
		// the environment is misconfigured. Return it unadorned so callers can recognize it and skip
		// the remediation hint.
		if ctx.Err() != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w. %s", err, credentialRemediation(a.LastCredentialSource(), cfg))
	}
	return resolved, nil
}

// credentialRemediation returns the check to suggest for the credential mechanism that was tried.
// Naming only that mechanism matters because the chain is first-match: telling an IRSA pod to
// verify IMDS reachability sends the operator down a path the Agent never took.
func credentialRemediation(source string, cfg pkgconfigmodel.Reader) string {
	switch source {
	case creds.SourceEnvironment:
		return "AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are set, so no other credential source was tried; check that they are valid and not expired"
	case creds.SourceWebIdentity:
		return "AWS_ROLE_ARN and AWS_WEB_IDENTITY_TOKEN_FILE are set, so IRSA was used; check that the token file is mounted and readable and that the role's trust policy allows this service account"
	case creds.SourceContainer:
		return "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI or AWS_CONTAINER_CREDENTIALS_FULL_URI is set, so ECS/EKS container credentials were used; check that the task role or Pod Identity association exists and that the credential endpoint is reachable"
	case creds.SourceIMDS:
		// Report the IMDS versions actually attempted rather than ec2_prefer_imdsv2 alone. The IMDS
		// helper allows v2 when either ec2_prefer_imdsv2 or ec2_imdsv2_transition_payload_enabled is
		// set (UseIMDSv2 in pkg/util/aws/creds/internal), and the latter defaults to true, so naming
		// only the first key tells a default-configured operator that v2 was not tried when it was.
		imdsVersions := "v1 only"
		if cfg.GetBool("ec2_prefer_imdsv2") || cfg.GetBool("ec2_imdsv2_transition_payload_enabled") {
			imdsVersions = "v2 then v1"
		}
		return fmt.Sprintf("no credential environment variables were set, so EC2 IMDS was used; check that an instance profile is attached and that IMDS is reachable (ec2_metadata_timeout=%dms, IMDS versions attempted=%s)",
			cfg.GetInt("ec2_metadata_timeout"), imdsVersions)
	default:
		// The source is unknown only if selection itself failed before recording one. Say nothing
		// specific rather than defaulting to IMDS advice, which would misdirect exactly the way the
		// old catch-all message did.
		return "the credential mechanism could not be determined"
	}
}

func (a *AWSAuth) getConnectionParameters() (string, string, string) {
	region := a.region
	var host string
	// Default to the default global STS Host (see here: https://docs.aws.amazon.com/general/latest/gr/sts.html)
	if region == "" {
		region = defaultRegion
		host = defaultStsHost
	} else {
		// If the region is not empty, use the regional STS host
		host = fmt.Sprintf(creds.RegionalStsHost, region)
	}
	stsFullURL := "https://" + host
	return stsFullURL, region, host
}

func (a *AWSAuth) getUserAgent() string {
	return "datadog-agent/" + version.AgentVersion
}

func (a *AWSAuth) generateAwsAuthData(ctx context.Context, orgUUID string, awsCredentials *creds.SecurityCredentials) (*signingData, error) {
	if orgUUID == "" {
		return nil, errors.New("missing org UUID")
	}
	if awsCredentials == nil || awsCredentials.AccessKeyID == "" || awsCredentials.SecretAccessKey == "" {
		return nil, errors.New("missing AWS credentials")
	}
	stsFullURL, region, host := a.getConnectionParameters()

	// Create the request body
	requestBody := getCallerIdentityBody
	bodyBytes := []byte(requestBody)

	// Calculate the payload hash manually
	payloadHashBytes := sha256.Sum256(bodyBytes)
	payloadHash := hex.EncodeToString(payloadHashBytes[:])

	// Create a seekable body reader
	bodyReader := bytes.NewReader(bodyBytes)

	// Create an HTTP request
	req, err := http.NewRequest(http.MethodPost, stsFullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers before signing
	req.Header.Set(contentTypeHeader, applicationForm)
	req.Header.Set(orgIDHeader, orgUUID)
	req.Header.Set("User-Agent", a.getUserAgent())
	req.ContentLength = int64(len(bodyBytes))
	req.Host = host

	// Create AWS credentials from our EC2 credentials
	awsCreds := aws.Credentials{
		AccessKeyID:     awsCredentials.AccessKeyID,
		SecretAccessKey: awsCredentials.SecretAccessKey,
		SessionToken:    awsCredentials.Token,
	}

	// Create the v4 signer
	signer := v4.NewSigner()

	// Sign the request
	// The orgIDHeader is already set on the request, so it will be included in the signature
	now := time.Now().UTC()
	err = signer.SignHTTP(ctx, awsCreds, req, payloadHash, service, region, now)
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	// Extract headers from the signed request
	headerMap := make(map[string][]string)
	for key, values := range req.Header {
		headerMap[key] = values
	}
	headerMap["Host"] = []string{host}

	// Marshal headers to JSON
	headersJSON, err := json.Marshal(headerMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal headers: %w", err)
	}

	return &signingData{
		headersEncoded: base64.StdEncoding.EncodeToString(headersJSON),
		bodyEncoded:    base64.StdEncoding.EncodeToString(bodyBytes),
		method:         http.MethodPost,
		urlEncoded:     base64.StdEncoding.EncodeToString([]byte(stsFullURL)),
	}, nil
}

// resolveCredentials selects the AWS credential provider matching the runtime
// environment, in the SDK's standard precedence but limited to the mechanisms a deployed Agent
// actually encounters: static env vars, IRSA web identity, ECS / EKS Pod Identity container
// credentials, and EC2 IMDS. It deliberately does not use config.LoadDefaultConfig, which would
// also link SSO, credential_process and shared-profile (~/.aws) support that the Agent does not
// need and which materially grows the binary. The static and container providers are from
// aws-sdk-go-v2; the web-identity and IMDS legs are handled directly (hand-rolled STS to avoid
// linking service/sts, and the Agent's IMDS helper to honor ec2_metadata_timeout). Only the
// selection is ours.
//
// Divergences from the SDK default chain are intentional and follow Agent conventions:
//   - IMDS is governed by Agent config (ec2_metadata_timeout, ec2_prefer_imdsv2), not the SDK's
//     IMDS env vars (AWS_EC2_METADATA_DISABLED / _V1_DISABLED / _SERVICE_ENDPOINT / _ENDPOINT_MODE),
//     which this path does not honor (the Agent honors none of them elsewhere either).
//   - the IRSA STS call uses the Agent's HTTP transport, so proxy / custom CA / TLS come from Agent
//     config and AWS_CA_BUNDLE is not consulted.
//   - only AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY are read for static creds (not the legacy
//     AWS_ACCESS_KEY / AWS_SECRET_KEY aliases), and shared-config / SSO / credential_process are
//     unsupported.
func (a *AWSAuth) resolveCredentials(ctx context.Context, cfg pkgconfigmodel.Reader) (*creds.SecurityCredentials, error) {
	// The mechanism is decided by the environment before any credential is fetched. Record it up
	// front so that a failure can name what was actually attempted: the chain is first-match, so
	// only one mechanism is ever tried and a message listing all four would misdirect.
	provider, source, err := a.credentialProvider(cfg)
	a.lastSource.Store(&source)
	if err != nil {
		return nil, fmt.Errorf("%s: provider setup failed: %w", source, err)
	}
	return a.resolveCredentialsFrom(ctx, provider, source)
}

// resolveCredentialsFrom retrieves and validates credentials from an already-selected provider.
// Split out from resolveCredentials so the retrieval and validation behavior can be tested against
// an injected provider without depending on the ambient environment.
func (a *AWSAuth) resolveCredentialsFrom(ctx context.Context, provider aws.CredentialsProvider, source string) (*creds.SecurityCredentials, error) {
	// Resolve once per call. Delegated auth re-runs this on each proof generation (startup and
	// every refresh interval), and the credentials it returns are valid for hours, so no
	// cross-call caching is needed.
	sdkCreds, err := provider.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	if sdkCreds.AccessKeyID == "" || sdkCreds.SecretAccessKey == "" {
		// Treat blank credentials as a failure rather than passing them on. The Agent's IMDS helper
		// unmarshals whatever JSON the metadata endpoint returns, so an error document answered with
		// a 200 yields an empty, error-free result; without this check the caller would log a
		// successful resolution and then fail to sign the proof for no visible reason.
		return nil, fmt.Errorf("%s: returned empty credentials", source)
	}

	// sdkCreds.Source is set by the provider that produced the credentials (ex:
	// DelegatedAuthWebIdentity, EC2RoleProvider). Logged at Info (once per key fetch, matching the
	// surrounding delegated-auth logs) so operators can confirm the credential source without
	// enabling debug; the status page reads it from lastSource for the same reason.
	log.Infof("delegated auth resolved AWS credentials via %s", sdkCreds.Source)

	return &creds.SecurityCredentials{
		AccessKeyID:     sdkCreds.AccessKeyID,
		SecretAccessKey: sdkCreds.SecretAccessKey,
		Token:           sdkCreds.SessionToken,
	}, nil
}

// credentialProvider warns about a half-configured credential source, then picks the provider
// for the region of the proof, see creds.CredentialProvider.
func (a *AWSAuth) credentialProvider(cfg pkgconfigmodel.Reader) (aws.CredentialsProvider, string, error) {
	// A half-configured mechanism is skipped rather than treated as an error, so the selection below
	// silently lands on a lower-precedence source and the proof gets signed as a different principal.
	// Say so, otherwise the only symptom is telemetry attributed to an unexpected identity.
	if incomplete := creds.IncompleteAWSCredentialEnv(); incomplete != "" {
		log.Warnf("delegated auth: %s, so that credential source is incomplete and was skipped; "+
			"falling back to the next source in the AWS precedence order. Set both variables if you "+
			"intended to use it.", incomplete)
	}

	return creds.CredentialProvider(cfg, a.resolveRegion())
}

// resolveRegion returns the region for the STS web-identity call, in the same precedence the SDK
// uses, with a final fallback so a region always exists: delegated_auth.aws.region (a.region),
// then AWS_REGION / AWS_DEFAULT_REGION, then defaultRegion. This mirrors the signing path and
// keeps the IRSA STS call working when no region is configured.
func (a *AWSAuth) resolveRegion() string {
	if a.region != "" {
		return a.region
	}
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		return r
	}
	return defaultRegion
}
