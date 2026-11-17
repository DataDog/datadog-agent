// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"

	"github.com/DataDog/datadog-agent/pkg/util/eks/identity"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// This file is a minimal client for the single EKS API call the resolver needs,
// DescribeCluster. It signs the request with the SigV4 signer and sends it with the HTTP client
// and credentials from the shared AWS configuration, both of which the Cluster Agent already
// links. It replaces the generated EKS service client, which added about 640 KiB to the
// Cluster Agent binary for one read-only call.
//
// The behavior mirrors aws-sdk-go-v2/service/eks v1.105.5: endpoint resolution (endpoints.go,
// including FIPS pseudo Regions and configured endpoints), REST-JSON error codes
// (restjson.SanitizeErrorCode), and the retryer's clock skew correction (aws/retry). Known
// differences: a FIPS pseudo Region is signed with the stripped Region, retries are one attempt
// instead of the SDK's three, and the response is reduced to the fields the resolver uses.
// TestResolveEndpointGolden pins the endpoints that client produces for every partition. Signing
// a request with aws/signer/v4 directly follows comp/core/delegatedauth/api/cloudauth/aws, which
// signs its STS GetCallerIdentity request the same way.

const (
	// eksSigningName is the SigV4 signing name of the EKS API.
	eksSigningName = "eks"
	// eksSDKID is the service ID used by the AWS SDKs for service-specific endpoint
	// configuration, such as the AWS_ENDPOINT_URL_EKS environment variable.
	eksSDKID = "EKS"
	// emptyPayloadHash is the SHA-256 of an empty body, the payload of a DescribeCluster GET.
	emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	// maxDescribeClusterResponseBytes bounds how much of a response is read. A DescribeCluster
	// answer is a few KiB; the bound only protects against a misbehaving endpoint.
	maxDescribeClusterResponseBytes = 1 << 20
)

// describedCluster holds the only DescribeCluster response fields the resolver uses.
type describedCluster struct {
	Name                 *string               `json:"name"`
	Arn                  *string               `json:"arn"`
	CertificateAuthority *certificateAuthority `json:"certificateAuthority"`
}

// certificateAuthority is the cluster's CA: base64 encoded PEM.
type certificateAuthority struct {
	Data *string `json:"data"`
}

// describeClusterOutput is the DescribeCluster response, reduced to the fields used.
type describeClusterOutput struct {
	Cluster *describedCluster `json:"cluster"`
}

type describeClusterAPI interface {
	DescribeCluster(ctx context.Context, clusterName string) (*describeClusterOutput, error)
}

// apiError is an error answer from the EKS API. It implements the error code and HTTP status
// interfaces that the AWS SDK retry classifier inspects, so failureTTL classifies it the same way
// it classified errors from the generated client.
type apiError struct {
	StatusCode int
	Code       string
	Message    string
	RequestID  string
	// skewChanged reports that the answer showed the request was signed with a wrong clock offset.
	skewChanged bool
}

func (e *apiError) Error() string {
	msg := fmt.Sprintf("api error %s (HTTP %d)", e.Code, e.StatusCode)
	if e.Message != "" {
		msg += ": " + e.Message
	}
	if e.RequestID != "" {
		msg += ", request id: " + e.RequestID
	}
	return msg
}

// ErrorCode returns the EKS error code, for example AccessDeniedException.
func (e *apiError) ErrorCode() string { return e.Code }

// ErrorMessage returns the EKS error message.
func (e *apiError) ErrorMessage() string { return e.Message }

// HTTPStatusCode returns the HTTP status of the error answer.
func (e *apiError) HTTPStatusCode() int { return e.StatusCode }

type signedDescribeClusterClient struct {
	cfg      aws.Config
	endpoint string
	signer   *v4.Signer
	now      func() time.Time
	// clockSkew is the server clock minus the local clock, in nanoseconds, from the Date header
	// of the last answer. It is added to the signing time of every later request.
	clockSkew atomic.Int64
}

func newSignedDescribeClusterClient(cfg aws.Config) describeClusterAPI {
	return &signedDescribeClusterClient{cfg: normalizeFIPSRegion(cfg), signer: v4.NewSigner(), now: time.Now}
}

// DescribeCluster calls GET /clusters/{name} on the EKS endpoint of the configured Region.
func (c *signedDescribeClusterClient) DescribeCluster(ctx context.Context, clusterName string) (*describeClusterOutput, error) {
	if c.cfg.Credentials == nil {
		return nil, errors.New("no AWS credentials configured")
	}
	endpoint := c.endpoint
	if endpoint == "" {
		var err error
		if endpoint, err = resolveEndpoint(ctx, c.cfg); err != nil {
			return nil, err
		}
	}
	base, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse EKS endpoint: %w", err)
	}
	// JoinPath keeps the path prefix of a configured endpoint. The generated client drops the
	// query, so this does too. The name is checked here as well as by the caller, so it can only
	// hold [A-Za-z0-9_-].
	if !identity.IsValidClusterName(clusterName) {
		return nil, ErrInvalidClusterName
	}
	base.RawQuery, base.ForceQuery = "", false
	// #nosec G125 -- clusterName is validated above
	reqURL := base.JoinPath("clusters", clusterName).String() //nolint:gosec // G125, see above

	// The retry policy is deliberately small: one re-signed attempt after a clock skew error,
	// and one more attempt after a transport error, a 429 or a 5xx when there is time left. The rest
	// of the policy is the caller's failure cache, which retries transient failures after 30
	// seconds instead of retrying in a loop while node Agents wait.
	skewCorrected, retried := false, false
	for {
		out, err := c.send(ctx, reqURL)
		if err == nil {
			return out, nil
		}
		var apiErr *apiError
		if !skewCorrected && errors.As(err, &apiErr) && apiErr.skewChanged && clockSkewCodes[apiErr.Code] {
			// Re-sign once with the corrected time, as the SDK's retryer does.
			skewCorrected = true
			continue
		}
		if !retried && isRetryable(err) && hasTimeForRetry(ctx) {
			retried = true
			continue
		}
		return nil, err
	}
}

// minRetryTime is the time an attempt needs to be worth starting.
const minRetryTime = 500 * time.Millisecond

func hasTimeForRetry(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	deadline, ok := ctx.Deadline()
	return !ok || time.Until(deadline) > minRetryTime
}

// transportError is a failure to get an HTTP answer at all (connection refused or reset, DNS,
// TLS, or a body cut short). It unwraps so the SDK retry classifier still sees the cause.
type transportError struct{ err error }

func (e *transportError) Error() string { return e.err.Error() }
func (e *transportError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var tErr *transportError
	if errors.As(err, &tErr) {
		return true
	}
	var apiErr *apiError
	return errors.As(err, &apiErr) && (apiErr.StatusCode >= http.StatusInternalServerError || apiErr.StatusCode == http.StatusTooManyRequests)
}

// send signs and sends one DescribeCluster request.
func (c *signedDescribeClusterClient) send(ctx context.Context, reqURL string) (*describeClusterOutput, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build DescribeCluster request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	// Identify the caller in CloudTrail the way the Cluster Agent's Kubernetes client does, plus
	// the SDK version that signs the request. The SigV4 signer does not sign User-Agent.
	req.Header.Set("User-Agent", userAgent())

	creds, err := c.cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("retrieve AWS credentials: %w", err)
	}
	signingTime := c.now().Add(time.Duration(c.clockSkew.Load()))
	if err := c.signer.SignHTTP(ctx, creds, req, emptyPayloadHash, eksSigningName, c.cfg.Region, signingTime); err != nil {
		return nil, fmt.Errorf("sign DescribeCluster request: %w", err)
	}

	httpClient := c.cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, &transportError{err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDescribeClusterResponseBytes))
	if err != nil {
		return nil, &transportError{fmt.Errorf("read DescribeCluster response: %w", err)}
	}
	skewChanged := c.observeServerTime(resp.Header)
	if resp.StatusCode != http.StatusOK {
		apiErr := parseAPIError(resp.StatusCode, resp.Header, body)
		apiErr.skewChanged = skewChanged
		return nil, apiErr
	}

	out := &describeClusterOutput{}
	if err := json.Unmarshal(body, out); err != nil {
		return nil, fmt.Errorf("%w: decode DescribeCluster response: %v", ErrInvalidClusterIdentity, err)
	}
	return out, nil
}

// clockSkewCodes are the error codes the SDK's clock skew correction reacts to
// (aws-sdk-go-v2 aws/retry clockSkewCodes). Like the SDK, every code, AccessDeniedException
// included, triggers a re-signed attempt only when the response's Date header shows the
// request was signed with an offset more than skewThreshold off.
var clockSkewCodes = map[string]bool{
	"InvalidSignatureException": true,
	"SignatureDoesNotMatch":     true,
	"AuthFailure":               true,
	"RequestTimeTooSkewed":      true,
	"AccessDeniedException":     true,
}

// skewThreshold is the clock difference above which the signing time is corrected. It is the
// SDK's value (aws/retry skewThreshold); AWS rejects signatures more than 5 minutes off.
const skewThreshold = 4 * time.Minute

// observeServerTime records the offset between the server's Date header and the local clock
// after every answer, success or not, as the SDK does (aws/retry middleware.go, ClientSkew is
// stored from every response). It reports whether the offset the request was signed with was
// more than skewThreshold away from the observed one: that is both a fresh skew and a stale
// offset after the local clock was fixed (aws/retry wrapAsClockSkew).
func (c *signedDescribeClusterClient) observeServerTime(header http.Header) bool {
	server := serverTime(header)
	if server.IsZero() {
		return false
	}
	observed := server.Sub(c.now())
	signedWith := time.Duration(c.clockSkew.Swap(int64(observed)))
	diff := signedWith - observed
	return diff > skewThreshold || diff < -skewThreshold
}

// parseAPIError reads the error code the way the AWS SDK REST-JSON protocol does: from the
// X-Amzn-ErrorType header first, then from the code or __type body field (matched without case,
// like the SDK's JSON decoder), sanitized by sanitizeErrorCode. Without any code it is
// UnknownError, the SDK's default.
func parseAPIError(statusCode int, header http.Header, body []byte) *apiError {
	var payload struct {
		Code    string `json:"code"`
		Type    string `json:"__type"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &payload)

	code := header.Get("X-Amzn-ErrorType")
	if code == "" {
		code = payload.Code
	}
	if code == "" {
		code = payload.Type
	}
	code = sanitizeErrorCode(code)
	if code == "" {
		code = "UnknownError"
	}
	return &apiError{
		StatusCode: statusCode,
		Code:       code,
		Message:    payload.Message,
		RequestID:  header.Get("X-Amzn-RequestId"),
	}
}

func serverTime(header http.Header) time.Time {
	t, err := http.ParseTime(header.Get("Date"))
	if err != nil {
		return time.Time{}
	}
	return t
}

// sanitizeErrorCode mirrors restjson.SanitizeErrorCode in aws-sdk-go-v2: keep the text before
// the first ':' (dropping a URI suffix), then the text after the first '#' (dropping a
// namespace prefix).
func sanitizeErrorCode(code string) string {
	code, _, _ = strings.Cut(code, ":")
	if _, after, found := strings.Cut(code, "#"); found {
		code = after
	}
	return code
}

func userAgent() string {
	return "datadog-cluster-agent/" + version.AgentVersion + " aws-sdk-go-v2/" + aws.SDKVersion
}

// partitionEndpoint describes the DNS suffixes of an AWS partition. The table mirrors the
// partition metadata the AWS SDK resolves endpoints with.
type partitionEndpoint struct {
	name            string
	regionPattern   *regexp.Regexp
	dnsSuffix       string
	dualStackSuffix string
}

var partitions = []partitionEndpoint{
	{"aws", regexp.MustCompile(`^(us|eu|ap|sa|ca|me|af|il|mx)-\w+-\d+$`), "amazonaws.com", "api.aws"},
	{"aws-cn", regexp.MustCompile(`^cn-\w+-\d+$`), "amazonaws.com.cn", "api.amazonwebservices.com.cn"},
	{"aws-eusc", regexp.MustCompile(`^eusc-(de)-\w+-\d+$`), "amazonaws.eu", "api.amazonwebservices.eu"},
	{"aws-iso", regexp.MustCompile(`^us-iso-\w+-\d+$`), "c2s.ic.gov", "api.aws.ic.gov"},
	{"aws-iso-b", regexp.MustCompile(`^us-isob-\w+-\d+$`), "sc2s.sgov.gov", "api.aws.scloud"},
	{"aws-iso-e", regexp.MustCompile(`^eu-isoe-\w+-\d+$`), "cloud.adc-e.uk", "api.cloud-aws.adc-e.uk"},
	{"aws-iso-f", regexp.MustCompile(`^us-isof-\w+-\d+$`), "csp.hci.ic.gov", "api.aws.hci.ic.gov"},
	{"aws-us-gov", regexp.MustCompile(`^us-gov-\w+-\d+$`), "amazonaws.com", "api.aws"},
}

// hostLabelPattern is the Region check the SDK applies before building an endpoint host.
var hostLabelPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func partitionFor(region string) partitionEndpoint {
	// The Region patterns do not overlap, so the order of the table does not matter.
	for _, p := range partitions {
		if p.regionPattern.MatchString(region) {
			return p
		}
	}
	// Like the SDK, an unknown Region falls back to the aws partition.
	return awsPartition
}

// awsPartition is the fallback for Regions no pattern matches.
var awsPartition = partitions[0]

// resolveEndpoint returns the EKS endpoint for cfg with the same precedence as the generated
// client: a service-specific endpoint (AWS_ENDPOINT_URL_EKS or the shared config services
// section), then the global configured endpoint, then the Region and partition endpoint, honoring
// the FIPS and dual-stack settings.
func resolveEndpoint(ctx context.Context, cfg aws.Config) (string, error) {
	cfg = normalizeFIPSRegion(cfg)
	useFIPS, useDualStack := endpointVariants(ctx, cfg.ConfigSources)
	if endpoint, ok := configuredEndpoint(ctx, cfg); ok {
		// The generated client refuses these instead of ignoring the variant (endpoints.go
		// resolveResult cases 1 and 2), so a FIPS setting is never dropped silently.
		switch {
		case useFIPS:
			return "", errors.New("endpoint rule error, Invalid Configuration: FIPS and custom endpoint are not supported")
		case useDualStack:
			return "", errors.New("endpoint rule error, Invalid Configuration: Dualstack and custom endpoint are not supported")
		}
		return endpoint, nil
	}

	region := cfg.Region
	if region == "" || !hostLabelPattern.MatchString(region) {
		return "", fmt.Errorf("invalid AWS Region %q for the EKS endpoint", region)
	}
	p := partitionFor(region)

	switch {
	case useFIPS && useDualStack:
		return "https://eks-fips." + region + "." + p.dualStackSuffix, nil
	case useFIPS && p.name == "aws":
		return "https://fips.eks." + region + ".amazonaws.com", nil
	case useFIPS && p.name == "aws-us-gov":
		// GovCloud EKS endpoints are FIPS endpoints.
		return "https://eks." + region + ".amazonaws.com", nil
	case useFIPS:
		return "https://eks-fips." + region + "." + p.dnsSuffix, nil
	case useDualStack:
		return "https://eks." + region + "." + p.dualStackSuffix, nil
	default:
		return "https://eks." + region + "." + p.dnsSuffix, nil
	}
}

// normalizeFIPSRegion handles the FIPS pseudo Regions (us-east-1-fips, fips-us-east-1) the way
// the generated client does (endpoints.go finalizeClientEndpointResolverOptions): -fips-, fips-
// and -fips are matched and removed anywhere in the Region, and the FIPS endpoint is forced on.
// One difference: the stripped Region is also used for the signature and the ARN check, while
// the generated client signs with the pseudo Region as given.
func normalizeFIPSRegion(cfg aws.Config) aws.Config {
	const fipsInfix, fipsPrefix, fipsSuffix = "-fips-", "fips-", "-fips"
	region := cfg.Region
	if !strings.Contains(region, fipsInfix) && !strings.Contains(region, fipsPrefix) && !strings.Contains(region, fipsSuffix) {
		return cfg
	}
	cfg.Region = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(region, fipsInfix, "-"), fipsPrefix, ""), fipsSuffix, "")
	cfg.ConfigSources = append([]interface{}{forcedFIPS{}}, cfg.ConfigSources...)
	return cfg
}

// forcedFIPS is a configuration source that turns the FIPS endpoint on. It is placed first so it
// takes precedence over the loaded environment and shared configuration.
type forcedFIPS struct{}

func (forcedFIPS) GetUseFIPSEndpoint(context.Context) (aws.FIPSEndpointState, bool, error) {
	return aws.FIPSEndpointStateEnabled, true, nil
}

// configuredEndpoint mirrors the generated client's base endpoint resolution. The global
// AWS_ENDPOINT_URL is already loaded into cfg.BaseEndpoint; a service-specific setting overrides
// it unless only the global environment variable is set.
func configuredEndpoint(ctx context.Context, cfg aws.Config) (string, bool) {
	endpoint := aws.ToString(cfg.BaseEndpoint)

	_, global := os.LookupEnv("AWS_ENDPOINT_URL")
	_, service := os.LookupEnv("AWS_ENDPOINT_URL_EKS")
	if !global || service {
		if serviceEndpoint, ok := serviceBaseEndpoint(ctx, cfg.ConfigSources); ok {
			endpoint = serviceEndpoint
		}
	}
	return endpoint, endpoint != ""
}

func serviceBaseEndpoint(ctx context.Context, sources []interface{}) (string, bool) {
	// Like the SDK, the first source that sets the ignore flag decides it.
	for _, src := range sources {
		if p, ok := src.(interface {
			GetIgnoreConfiguredEndpoints(context.Context) (bool, bool, error)
		}); ok {
			ignore, found, err := p.GetIgnoreConfiguredEndpoints(ctx)
			if err != nil || found {
				if err == nil && ignore {
					return "", false
				}
				break
			}
		}
	}
	for _, src := range sources {
		if p, ok := src.(interface {
			GetServiceBaseEndpoint(context.Context, string) (string, bool, error)
		}); ok {
			endpoint, found, err := p.GetServiceBaseEndpoint(ctx, eksSDKID)
			if err != nil {
				return "", false
			}
			if found {
				return endpoint, endpoint != ""
			}
		}
	}
	return "", false
}

// endpointVariants reads the FIPS and dual-stack endpoint settings from the loaded configuration
// sources (environment, then shared config), first found wins, as the SDK does.
func endpointVariants(ctx context.Context, sources []interface{}) (useFIPS, useDualStack bool) {
	fipsFound, dualStackFound := false, false
	for _, src := range sources {
		if p, ok := src.(interface {
			GetUseFIPSEndpoint(context.Context) (aws.FIPSEndpointState, bool, error)
		}); ok && !fipsFound {
			if state, found, err := p.GetUseFIPSEndpoint(ctx); err == nil && found {
				fipsFound, useFIPS = true, state == aws.FIPSEndpointStateEnabled
			}
		}
		if p, ok := src.(interface {
			GetUseDualStackEndpoint(context.Context) (aws.DualStackEndpointState, bool, error)
		}); ok && !dualStackFound {
			if state, found, err := p.GetUseDualStackEndpoint(ctx); err == nil && found {
				dualStackFound, useDualStack = true, state == aws.DualStackEndpointStateEnabled
			}
		}
	}
	return useFIPS, useDualStack
}
