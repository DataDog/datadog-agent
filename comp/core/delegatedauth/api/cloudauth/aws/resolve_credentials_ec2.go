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
	"github.com/aws/aws-sdk-go-v2/credentials/endpointcreds"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
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
	// defaultWebIdentitySessionName is the RoleSessionName used when AWS_ROLE_SESSION_NAME is unset.
	defaultWebIdentitySessionName = "datadog-agent-workload-identity-federation"
	// webIdentityAPIVersion is the STS query-API version.
	webIdentityAPIVersion = "2011-06-15"
	// maxSTSResponseBytes bounds the STS response read to avoid unbounded memory use.
	maxSTSResponseBytes = 1 << 20
	// containerCredentialsTimeout bounds a container credential fetch so a local endpoint that
	// accepts the connection but stalls cannot hang the initial fetch or a background refresh.
	containerCredentialsTimeout = 10 * time.Second
)

// resolveCredentials (ec2 build) selects the AWS credential provider matching the runtime
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
func (a *AWSAuth) resolveCredentials(ctx context.Context, cfg pkgconfigmodel.Reader) *creds.SecurityCredentials {
	provider, err := a.credentialProvider(cfg)
	if err != nil {
		log.Warnf("AWS credential provider setup failed: %v", err)
		return &creds.SecurityCredentials{}
	}

	// Resolve once per call. Delegated auth re-runs this on each proof generation (startup and
	// every refresh interval), and the credentials it returns are valid for hours, so no
	// cross-call caching is needed.
	sdkCreds, err := provider.Retrieve(ctx)
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
func (a *AWSAuth) credentialProvider(cfg pkgconfigmodel.Reader) (aws.CredentialsProvider, error) {
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
		// This is the only outbound external call in credential resolution (static needs no
		// network; container/IMDS use link-local addresses), so it uses the Agent's configured
		// HTTP transport for proxy / custom CA / TLS settings, matching the intake-key call.
		// RoleSessionName follows the standard AWS_ROLE_SESSION_NAME env var (as the SDK does),
		// falling back to our default, so a role trust policy that conditions on sts:RoleSessionName
		// still assumes correctly.
		sessionName := os.Getenv("AWS_ROLE_SESSION_NAME")
		if sessionName == "" {
			sessionName = defaultWebIdentitySessionName
		}
		return &webIdentityProvider{
			roleARN:     os.Getenv("AWS_ROLE_ARN"),
			tokenFile:   os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"),
			sessionName: sessionName,
			stsURL:      "https://" + fmt.Sprintf(regionalStsHost, a.resolveRegion()) + "/",
			client:      &http.Client{Timeout: 10 * time.Second, Transport: httputils.CreateHTTPTransport(cfg)},
		}, nil

	case creds.HasAWSContainerCredentialsInEnvironment():
		return containerCredentialsProvider()

	default:
		// No env / IRSA / container credentials: fall back to the EC2 instance role via IMDS.
		// Use the Agent's IMDS helper (creds.GetSecurityCredentials) rather than a default aws-sdk
		// IMDS client so the call honors ec2_metadata_timeout and the Agent's IMDSv2 configuration
		// (ec2_prefer_imdsv2 / ec2_imdsv2_transition_payload_enabled), matching every other Agent
		// IMDS access.
		return imdsProvider{fetch: creds.GetSecurityCredentials}, nil
	}
}

// imdsProvider resolves EC2 instance-role credentials through the Agent's IMDS helper, which
// applies the Agent's ec2_metadata_timeout and IMDSv2 configuration. It implements
// aws.CredentialsProvider so it slots into the same resolution path as the other providers. fetch
// is creds.GetSecurityCredentials in production and is injected in tests.
type imdsProvider struct {
	fetch func(ctx context.Context) (*creds.SecurityCredentials, error)
}

// Retrieve fetches the instance-role credentials and maps them to aws.Credentials.
func (p imdsProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	c, err := p.fetch(ctx)
	if err != nil {
		return aws.Credentials{}, err
	}
	return aws.Credentials{
		AccessKeyID:     c.AccessKeyID,
		SecretAccessKey: c.SecretAccessKey,
		SessionToken:    c.Token,
		Source:          "DelegatedAuthIMDS",
	}, nil
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
	roleARN     string
	tokenFile   string
	sessionName string
	stsURL      string
	client      *http.Client
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
		"RoleSessionName":  {p.sessionName},
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

// containerCredentialsEndpoint resolves the container-credential endpoint following the AWS
// contract: AWS_CONTAINER_CREDENTIALS_RELATIVE_URI (required to be a path and resolved against the
// trusted ECS endpoint) takes precedence, falling back to AWS_CONTAINER_CREDENTIALS_FULL_URI
// (validated to a loopback/ECS/EKS host). Relative wins because an ECS task carries the
// ECS-injected relative URI and may also see a stale full URI from the image or environment.
func containerCredentialsEndpoint() (string, error) {
	if relative := os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"); relative != "" {
		// The relative URI is joined onto the trusted ECS endpoint, so it must be a path. A value
		// that does not start with "/" could alter the URL authority and bypass the host allowlist
		// (ex: "@attacker.example/creds" -> "http://169.254.170.2@attacker.example/creds", whose
		// host is attacker.example), leaking the container authorization token to an arbitrary host.
		if !strings.HasPrefix(relative, "/") {
			return "", fmt.Errorf("container credentials relative URI must be a path starting with %q: %q", "/", relative)
		}
		endpoint := ecsContainerEndpoint + relative
		// Defense in depth: confirm the joined URL still resolves to an allowed ECS/EKS/loopback host.
		if err := validateContainerEndpoint(endpoint); err != nil {
			return "", err
		}
		return endpoint, nil
	}
	endpoint := os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI")
	if err := validateContainerEndpoint(endpoint); err != nil {
		return "", err
	}
	return endpoint, nil
}

// containerCredentialsProvider builds the ECS task-role / EKS Pod Identity provider from the
// standard container-credential env vars, mirroring the AWS SDK: AWS_CONTAINER_CREDENTIALS_RELATIVE_URI
// (resolved against the ECS endpoint) takes precedence, falling back to
// AWS_CONTAINER_CREDENTIALS_FULL_URI (validated to a loopback/ECS/EKS host), with an optional
// authorization token from AWS_CONTAINER_AUTHORIZATION_TOKEN or AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE.
// It uses a proxy-less HTTP client (see containerCredentialsHTTPClient) so the local credential
// request is never routed through an environment proxy.
func containerCredentialsProvider() (aws.CredentialsProvider, error) {
	endpoint, err := containerCredentialsEndpoint()
	if err != nil {
		return nil, err
	}

	return endpointcreds.New(endpoint, func(o *endpointcreds.Options) {
		o.HTTPClient = containerCredentialsHTTPClient()
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

// containerCredentialsHTTPClient returns an HTTP client that never uses a proxy. The supported
// container credential endpoints are link-local (ECS 169.254.170.2 / EKS Pod Identity
// 169.254.170.23) or loopback, which a forward proxy cannot reach; routing the request through
// HTTP_PROXY/http_proxy would break the fetch and send AWS_CONTAINER_AUTHORIZATION_TOKEN to the
// proxy. The default aws-sdk client's transport consults environment proxies, so this overrides it.
// The client also carries a timeout so a stalled endpoint cannot hang the fetch or a refresh.
func containerCredentialsHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Timeout: containerCredentialsTimeout, Transport: transport}
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
