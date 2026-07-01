// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ec2

package aws

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/credentials/endpointcreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"

	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Container credential endpoint and the hosts the SDK allows for an http
// AWS_CONTAINER_CREDENTIALS_FULL_URI: loopback plus the link-local ECS and EKS Pod Identity
// addresses. Anything else is rejected to avoid leaking credentials to an arbitrary host.
var (
	ecsContainerEndpoint = "http://169.254.170.2"
	ecsContainerIPv4     = net.IP{169, 254, 170, 2}
	eksContainerIPv4     = net.IP{169, 254, 170, 23}
	eksContainerIPv6     = net.IP{0xFD, 0, 0x0E, 0xC2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x23}
)

const (
	// webIdentitySessionName is the RoleSessionName sent with AssumeRoleWithWebIdentity. It is
	// informational (appears in the assumed-role ARN / CloudTrail); mappings key on the role.
	webIdentitySessionName = "datadog-agent-workload-identity-federation"
	// webIdentityAPIVersion is the STS query-API version.
	webIdentityAPIVersion = "2011-06-15"
	// maxSTSResponseBytes bounds the STS response read to avoid unbounded memory use.
	maxSTSResponseBytes = 1 << 20
)

// resolveCredentials (ec2 build) selects the AWS credential provider matching the runtime
// environment, in the SDK's standard precedence but limited to the mechanisms a deployed Agent
// actually encounters: static env vars, IRSA web identity, ECS / EKS Pod Identity container
// credentials, and EC2 IMDS. It deliberately does not use config.LoadDefaultConfig, which would
// also link SSO, credential_process and shared-profile (~/.aws) support that the Agent does not
// need and which materially grows the binary. Each provider is from aws-sdk-go-v2 (except the
// web-identity provider, hand-rolled to avoid linking service/sts); only the selection is ours.
func (a *AWSAuth) resolveCredentials(ctx context.Context) *creds.SecurityCredentials {
	provider, err := a.credentialProvider()
	if err != nil {
		log.Warnf("AWS credential provider setup failed: %v", err)
		return &creds.SecurityCredentials{}
	}

	// Wrap in a cache so providers that return expiring credentials (web identity, container,
	// IMDS instance role) refresh correctly; LoadDefaultConfig did this for us.
	sdkCreds, err := aws.NewCredentialsCache(provider).Retrieve(ctx)
	if err != nil {
		log.Warnf("AWS credential retrieval failed: %v", err)
		return &creds.SecurityCredentials{}
	}

	// sdkCreds.Source is set by the provider that produced the credentials (ex:
	// DelegatedAuthWebIdentity, EC2RoleProvider), naming which environment matched. Logged at Info
	// (once per key fetch, matching the surrounding delegated-auth logs) so operators can confirm
	// the credential source without enabling debug.
	log.Infof("delegated auth resolved AWS credentials via %s", sdkCreds.Source)

	return &creds.SecurityCredentials{
		AccessKeyID:     sdkCreds.AccessKeyID,
		SecretAccessKey: sdkCreds.SecretAccessKey,
		Token:           sdkCreds.SessionToken,
	}
}

// credentialProvider picks the credential provider for the current environment, matching the AWS
// SDK default-chain precedence: static env vars, then IRSA web identity, then container
// credentials (ECS / EKS Pod Identity), then EC2 IMDS instance role.
func (a *AWSAuth) credentialProvider() (aws.CredentialsProvider, error) {
	switch {
	case creds.HasAWSCredentialsInEnvironment():
		return credentials.NewStaticCredentialsProvider(
			os.Getenv("AWS_ACCESS_KEY_ID"),
			os.Getenv("AWS_SECRET_ACCESS_KEY"),
			os.Getenv("AWS_SESSION_TOKEN"),
		), nil

	case creds.HasAWSWorkloadIdentityInEnvironment():
		// IRSA: exchange the projected web-identity token via STS AssumeRoleWithWebIdentity. We
		// call STS directly (a plain unsigned POST + XML parse, mirroring the hand-rolled
		// GetCallerIdentity proof) rather than through service/sts, which would link the full STS
		// client and materially grow the binary. resolveRegion always yields a region (defaulting
		// to defaultRegion) so an IRSA-only pod with no AWS_REGION/AWS_DEFAULT_REGION still works.
		return &webIdentityProvider{
			roleARN:   os.Getenv("AWS_ROLE_ARN"),
			tokenFile: os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"),
			stsURL:    "https://" + fmt.Sprintf(regionalStsHost, a.resolveRegion()) + "/",
			client:    &http.Client{Timeout: 10 * time.Second},
		}, nil

	case creds.HasAWSContainerCredentialsInEnvironment():
		return containerCredentialsProvider()

	default:
		return ec2rolecreds.New(func(o *ec2rolecreds.Options) {
			o.Client = imds.New(imds.Options{})
		}), nil
	}
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

// webIdentityProvider retrieves credentials via STS AssumeRoleWithWebIdentity using the projected
// web-identity token (IRSA / EKS). It implements aws.CredentialsProvider. The call is
// unauthenticated apart from the token, so unlike the GetCallerIdentity proof it needs no SigV4
// signing; we POST the query-API form and parse the XML response.
type webIdentityProvider struct {
	roleARN   string
	tokenFile string
	stsURL    string
	client    *http.Client
}

// assumeRoleWithWebIdentityResponse is the subset of the STS XML response we consume.
type assumeRoleWithWebIdentityResponse struct {
	Result struct {
		Credentials struct {
			AccessKeyID     string    `xml:"AccessKeyId"`
			SecretAccessKey string    `xml:"SecretAccessKey"`
			SessionToken    string    `xml:"SessionToken"`
			Expiration      time.Time `xml:"Expiration"`
		} `xml:"Credentials"`
	} `xml:"AssumeRoleWithWebIdentityResult"`
}

// Retrieve exchanges the web-identity token for temporary credentials.
func (p *webIdentityProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	if p.roleARN == "" || p.tokenFile == "" {
		return aws.Credentials{}, errors.New("web identity: AWS_ROLE_ARN and AWS_WEB_IDENTITY_TOKEN_FILE must be set")
	}
	token, err := os.ReadFile(p.tokenFile)
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("web identity: read token file %s: %w", p.tokenFile, err)
	}

	form := url.Values{
		"Action":           {"AssumeRoleWithWebIdentity"},
		"Version":          {webIdentityAPIVersion},
		"RoleArn":          {p.roleARN},
		"RoleSessionName":  {webIdentitySessionName},
		"WebIdentityToken": {strings.TrimSpace(string(token))},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.stsURL, strings.NewReader(form.Encode()))
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("web identity: build STS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("web identity: STS request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSTSResponseBytes))
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("web identity: read STS response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return aws.Credentials{}, fmt.Errorf("web identity: STS returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var parsed assumeRoleWithWebIdentityResponse
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return aws.Credentials{}, fmt.Errorf("web identity: parse STS response: %w", err)
	}
	c := parsed.Result.Credentials
	if c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return aws.Credentials{}, errors.New("web identity: STS response missing credentials")
	}

	return aws.Credentials{
		AccessKeyID:     c.AccessKeyID,
		SecretAccessKey: c.SecretAccessKey,
		SessionToken:    c.SessionToken,
		Source:          "DelegatedAuthWebIdentity",
		CanExpire:       !c.Expiration.IsZero(),
		Expires:         c.Expiration,
	}, nil
}

// containerCredentialsProvider builds the ECS task-role / EKS Pod Identity provider from the
// standard container-credential env vars, mirroring the AWS SDK: AWS_CONTAINER_CREDENTIALS_FULL_URI
// (validated to a loopback/ECS/EKS host) or AWS_CONTAINER_CREDENTIALS_RELATIVE_URI (resolved
// against the ECS endpoint), with an optional authorization token from
// AWS_CONTAINER_AUTHORIZATION_TOKEN or AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE.
func containerCredentialsProvider() (aws.CredentialsProvider, error) {
	endpoint := os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI")
	if endpoint != "" {
		if err := validateContainerEndpoint(endpoint); err != nil {
			return nil, err
		}
	} else {
		endpoint = ecsContainerEndpoint + os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI")
	}

	return endpointcreds.New(endpoint, func(o *endpointcreds.Options) {
		if token := os.Getenv("AWS_CONTAINER_AUTHORIZATION_TOKEN"); token != "" {
			o.AuthorizationToken = token
		}
		if path := os.Getenv("AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE"); path != "" {
			o.AuthorizationTokenProvider = endpointcreds.TokenProviderFunc(func() (string, error) {
				contents, err := os.ReadFile(path)
				if err != nil {
					return "", fmt.Errorf("failed to read authorization token from %s: %w", path, err)
				}
				return string(contents), nil
			})
		}
	}), nil
}

// validateContainerEndpoint guards an http AWS_CONTAINER_CREDENTIALS_FULL_URI: the host must be
// loopback or a known ECS/EKS link-local address, so credentials are not sent to an arbitrary
// host. https endpoints are trusted as-is, matching the SDK.
func validateContainerEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid container credentials URI: %w", err)
	}
	host := parsed.Hostname()
	if host == "" {
		return errors.New("invalid container credentials URI: missing host")
	}
	if parsed.Scheme != "http" {
		return nil
	}
	allowed, err := isAllowedContainerHost(host)
	if err != nil {
		return fmt.Errorf("failed to resolve container credentials host %q: %w", host, err)
	}
	if !allowed {
		return fmt.Errorf("container credentials host %q is not loopback/ECS/EKS", host)
	}
	return nil
}

func isAllowedContainerHost(host string) (bool, error) {
	if ip := net.ParseIP(host); ip != nil {
		return isAllowedContainerIP(ip), nil
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		return false, err
	}
	for _, addr := range addrs {
		if ip := net.ParseIP(addr); ip == nil || !isAllowedContainerIP(ip) {
			return false, nil
		}
	}
	return len(addrs) > 0, nil
}

func isAllowedContainerIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.Equal(ecsContainerIPv4) ||
		ip.Equal(eksContainerIPv4) ||
		ip.Equal(eksContainerIPv6)
}
