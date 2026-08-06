// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	"context"
	"encoding/json"
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

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// awsCredentials is a resolved AWS credential set. It replaces aws-sdk-go-v2's aws.Credentials,
// which every leg below now populates by hand: with the last SDK provider gone (see
// staticProvider and containerProvider) the type carried no SDK behavior, only the SDK's
// package graph. Field names and semantics are unchanged so the providers read the same.
type awsCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string

	// Source names the provider that produced these credentials (ex: DelegatedAuthIMDS). It is
	// logged so operators can confirm which mechanism was used.
	Source string

	// CanExpire and Expires carry the expiry reported by the issuing endpoint, where there is one.
	CanExpire bool
	Expires   time.Time
}

// credentialsProvider resolves credentials for one mechanism. It replaces
// aws-sdk-go-v2's aws.CredentialsProvider, which this package only ever used to hold its own
// provider implementations behind a common interface.
type credentialsProvider interface {
	Retrieve(ctx context.Context) (awsCredentials, error)
}

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
	// containerCredentialsMaxResponseBytes bounds the credential document read. The real document is
	// a few hundred bytes; this leaves generous headroom while capping a misbehaving endpoint.
	containerCredentialsMaxResponseBytes = 64 << 10
	// containerCredentialsMaxAttempts and containerCredentialsRetryBudget bound the retries for a
	// transient container-credential failure. The endpoint is link-local, so a healthy one answers in
	// single-digit milliseconds and a genuinely transient blip clears well inside the budget; the
	// budget in turn keeps a stalled endpoint from multiplying containerCredentialsTimeout.
	containerCredentialsMaxAttempts = 3
	containerCredentialsRetryBudget = 3 * time.Second
	// containerCredentialsRetryDelay spaces the retries. Fixed rather than exponential: the whole
	// sequence is capped at containerCredentialsRetryBudget, so backing off adds no headroom.
	containerCredentialsRetryDelay = 200 * time.Millisecond
)

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
func (a *AWSAuth) resolveCredentialsFrom(ctx context.Context, provider credentialsProvider, source string) (*creds.SecurityCredentials, error) {
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

// credentialProvider picks the credential provider for the current environment, matching the AWS
// SDK default-chain precedence: static env vars, then IRSA web identity, then container
// credentials (ECS / EKS Pod Identity), then EC2 IMDS instance role. It also returns the name of
// the mechanism it selected, so callers can attribute a failure to the one leg that was tried.
func (a *AWSAuth) credentialProvider(cfg pkgconfigmodel.Reader) (credentialsProvider, string, error) {
	// A half-configured mechanism is skipped rather than treated as an error, so the selection below
	// silently lands on a lower-precedence source and the proof gets signed as a different principal.
	// Say so, otherwise the only symptom is telemetry attributed to an unexpected identity.
	if incomplete := creds.IncompleteAWSCredentialEnv(); incomplete != "" {
		log.Warnf("delegated auth: %s, so that credential source is incomplete and was skipped; "+
			"falling back to the next source in the AWS precedence order. Set both variables if you "+
			"intended to use it.", incomplete)
	}

	switch {
	case creds.HasAWSCredentialsInEnvironment():
		return staticProvider{awsCredentials{
			AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
			SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
			Source:          "DelegatedAuthStaticEnvironment",
		}}, creds.SourceEnvironment, nil

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
		}, creds.SourceWebIdentity, nil

	case creds.HasAWSContainerCredentialsInEnvironment():
		provider, err := containerCredentialsProvider()
		return provider, creds.SourceContainer, err

	default:
		// No env / IRSA / container credentials: fall back to the EC2 instance role via IMDS.
		// Use the Agent's IMDS helper (creds.GetSecurityCredentials) rather than a default aws-sdk
		// IMDS client so the call honors ec2_metadata_timeout and the Agent's IMDSv2 configuration
		// (ec2_prefer_imdsv2 / ec2_imdsv2_transition_payload_enabled), matching every other Agent
		// IMDS access.
		return imdsProvider{fetch: creds.GetSecurityCredentials}, creds.SourceIMDS, nil
	}
}

// staticProvider returns fixed credentials read from the environment. It stands in for
// credentials.NewStaticCredentialsProvider so that aws-sdk-go-v2/credentials is not linked for a
// value the caller already has in hand.
type staticProvider struct{ creds awsCredentials }

// Retrieve returns the fixed credentials.
func (p staticProvider) Retrieve(context.Context) (awsCredentials, error) { return p.creds, nil }

// imdsProvider resolves EC2 instance-role credentials through the Agent's IMDS helper, which
// applies the Agent's ec2_metadata_timeout and IMDSv2 configuration. It implements
// credentialsProvider so it slots into the same resolution path as the other providers. fetch
// is creds.GetSecurityCredentials in production and is injected in tests.
type imdsProvider struct {
	fetch func(ctx context.Context) (*creds.SecurityCredentials, error)
}

// Retrieve fetches the instance-role credentials and maps them to awsCredentials.
func (p imdsProvider) Retrieve(ctx context.Context) (awsCredentials, error) {
	c, err := p.fetch(ctx)
	if err != nil {
		return awsCredentials{}, err
	}
	return awsCredentials{
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
// web-identity token (IRSA / EKS). It implements credentialsProvider. The call is
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
func (p *webIdentityProvider) Retrieve(ctx context.Context) (awsCredentials, error) {
	if p.roleARN == "" || p.tokenFile == "" {
		return awsCredentials{}, errors.New("AWS_ROLE_ARN and AWS_WEB_IDENTITY_TOKEN_FILE must be set")
	}
	token, err := os.ReadFile(p.tokenFile)
	if err != nil {
		return awsCredentials{}, fmt.Errorf("read token file %s: %w", p.tokenFile, err)
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
		return awsCredentials{}, fmt.Errorf("build STS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return awsCredentials{}, fmt.Errorf("STS request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSTSResponseBytes))
	if err != nil {
		return awsCredentials{}, fmt.Errorf("read STS response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return awsCredentials{}, fmt.Errorf("STS returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var parsed assumeRoleWithWebIdentityResponse
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return awsCredentials{}, fmt.Errorf("parse STS response: %w", err)
	}
	c := parsed.Result.Credentials
	if c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return awsCredentials{}, errors.New("STS response missing credentials")
	}

	return awsCredentials{
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
func containerCredentialsProvider() (credentialsProvider, error) {
	endpoint, err := containerCredentialsEndpoint()
	if err != nil {
		return nil, err
	}

	p := &containerProvider{endpoint: endpoint, client: containerCredentialsHTTPClient()}
	if token := os.Getenv("AWS_CONTAINER_AUTHORIZATION_TOKEN"); token != "" {
		p.token = func() (string, error) { return token, nil }
	}
	// The token file wins over the literal token, and is re-read on every Retrieve: EKS Pod Identity
	// rotates the file, so a value cached at construction would go stale and start returning 401.
	if path := os.Getenv("AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE"); path != "" {
		p.token = func() (string, error) {
			contents, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("failed to read authorization token from %s: %w", path, err)
			}
			return strings.TrimSpace(string(contents)), nil
		}
	}
	return p, nil
}

// containerProvider fetches ECS task-role / EKS Pod Identity credentials with a plain GET against
// the link-local credential endpoint, mirroring the hand-rolled IMDS and web-identity providers
// above. The response is the documented container-credential JSON document.
//
// We do this rather than use credentials/endpointcreds because that package reaches the same
// one-shot GET through the full smithy-go middleware and retry stack, which measured at ~320 KiB of
// code and reflection metadata in the trace-agent (about 80% of this feature's total size cost) for
// a single unauthenticated request to 169.254.170.2. Everything endpointcreds would have
// contributed beyond the request itself, the endpoint allowlist, the proxy-less client and the
// authorization token handling, is already implemented here.
type containerProvider struct {
	endpoint string
	client   *http.Client
	// token supplies the Authorization header value, or is nil when the endpoint needs no token.
	token func() (string, error)
}

// Retrieve fetches the container credential document, retrying a transient failure a few times
// before giving up.
//
// The retries matter because the caller's fallback is slow: a failed fetch propagates to the
// delegated-auth refresh loop, whose backoff starts at delegated_auth.refresh_interval_mins (60 by
// default), so without them one transient response at startup leaves the Agent with no API key for
// about an hour. credentials/endpointcreds, which this provider replaced, got equivalent retries
// from the SDK's standard retryer.
//
// Attempts are bounded by both a count and an elapsed budget. The budget is what keeps this from
// trading the hour-long outage for a startup stall: containerCredentialsTimeout is per attempt, so
// against a wedged endpoint the first attempt alone exhausts the budget and Retrieve returns after
// one timeout rather than three.
func (p *containerProvider) Retrieve(ctx context.Context) (awsCredentials, error) {
	deadline := time.Now().Add(containerCredentialsRetryBudget)

	var lastErr error
	for attempt := 1; attempt <= containerCredentialsMaxAttempts; attempt++ {
		c, retryable, err := p.retrieveOnce(ctx)
		if err == nil {
			return c, nil
		}
		lastErr = err

		if !retryable || attempt == containerCredentialsMaxAttempts || !time.Now().Before(deadline) {
			break
		}
		log.Debugf("container credentials attempt %d failed, retrying: %v", attempt, err)

		select {
		case <-ctx.Done():
			return awsCredentials{}, ctx.Err()
		case <-time.After(containerCredentialsRetryDelay):
		}
	}
	return awsCredentials{}, lastErr
}

// retrieveOnce performs a single fetch. The bool reports whether the failure looks transient, which
// is the same set credentials/endpointcreds retried through the SDK retryer: connection-level
// errors, 5xx, and 429. A non-2xx outside that set (ex: 403 from a wrong authorization token) is a
// configuration problem that retrying cannot fix, and a malformed document is not transient either.
func (p *containerProvider) retrieveOnce(ctx context.Context) (awsCredentials, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint, nil)
	if err != nil {
		return awsCredentials{}, false, fmt.Errorf("build container credentials request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if p.token != nil {
		token, err := p.token()
		if err != nil {
			return awsCredentials{}, false, err
		}
		req.Header.Set("Authorization", token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		// Connection refused, DNS, TLS, or the per-attempt timeout: all transient from here.
		return awsCredentials{}, true, fmt.Errorf("container credentials request failed: %w", err)
	}
	defer resp.Body.Close()

	// Cap the read: the endpoint is link-local and trusted, but a wedged or misbehaving one should
	// not be able to make the Agent allocate without bound.
	body, err := io.ReadAll(io.LimitReader(resp.Body, containerCredentialsMaxResponseBytes))
	if err != nil {
		// A truncated body is usually the connection dropping mid-response.
		return awsCredentials{}, true, fmt.Errorf("read container credentials response: %w", err)
	}
	// Accept any 2xx, as credentials/endpointcreds does. The endpoint is documented to answer 200,
	// but matching the reference implementation's range avoids rejecting a valid credential document
	// over a status code we did not anticipate.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryable := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return awsCredentials{}, retryable, fmt.Errorf("container credentials endpoint returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var doc struct {
		AccessKeyID     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
		Token           string `json:"Token"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return awsCredentials{}, false, fmt.Errorf("parse container credentials response: %w", err)
	}

	// Blank fields are caught by resolveCredentialsFrom, which reports them against this source.
	return awsCredentials{
		AccessKeyID:     doc.AccessKeyID,
		SecretAccessKey: doc.SecretAccessKey,
		SessionToken:    doc.Token,
		Source:          "DelegatedAuthContainer",
	}, false, nil
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
