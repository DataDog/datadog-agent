// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package query provides a minimal SigV4-signed HTTP client for AWS Query XML
// and EC2 Query XML APIs without linking generated aws-sdk-go-v2 service clients.
package query

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

const (
	// defaultMaxBodyBytes bounds response reads to avoid unbounded memory usage while
	// comfortably accommodating large paginated Describe* XML payloads.
	defaultMaxBodyBytes = 10 << 20 // 10 MiB

	// maxErrorSnippetBytes caps raw non-XML error bodies included in error messages.
	maxErrorSnippetBytes = 512
)

// Client sends SigV4-signed AWS Query XML / EC2 Query XML requests.
type Client struct {
	service      string
	apiVersion   string
	region       string
	endpoint     string
	credentials  aws.CredentialsProvider
	httpClient   aws.HTTPClient
	signer       *v4.Signer
	nowFunc      func() time.Time
	maxBodyBytes int64
}

// APIError represents an error returned by an AWS Query XML or EC2 Query XML endpoint.
type APIError struct {
	Service    string
	Action     string
	StatusCode int
	Code       string
	Message    string
	RequestID  string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "AWS %s %s failed (HTTP %d)", e.Service, e.Action, e.StatusCode)
	if e.Code != "" {
		fmt.Fprintf(&b, ": %s", e.Code)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " (request ID: %s)", e.RequestID)
	}
	return b.String()
}

// Duck-typed interfaces matching the exported provider methods on aws-sdk-go-v2/config
// ConfigSources (EnvConfig, SharedConfig, LoadOptions) without importing SDK internal packages.
type serviceBaseEndpointProvider interface {
	GetServiceBaseEndpoint(ctx context.Context, sdkID string) (string, bool, error)
}

type useFIPSEndpointProvider interface {
	GetUseFIPSEndpoint(ctx context.Context) (aws.FIPSEndpointState, bool, error)
}

type useDualStackEndpointProvider interface {
	GetUseDualStackEndpoint(ctx context.Context) (aws.DualStackEndpointState, bool, error)
}

type ignoreConfiguredEndpointsProvider interface {
	GetIgnoreConfiguredEndpoints(ctx context.Context) (bool, bool, error)
}

// NewClient constructs a Client for the given AWS service (signing name and subdomain,
// e.g. "ec2" or "rds"), SDK ID (used for AWS_ENDPOINT_URL_<SDKID>, e.g. "EC2" or "RDS"),
// and Query API version (e.g. "2016-11-15" or "2014-10-31").
func NewClient(cfg aws.Config, service, sdkID, apiVersion string) (*Client, error) {
	if service == "" {
		return nil, errors.New("AWS service name is required")
	}
	if apiVersion == "" {
		return nil, errors.New("AWS API version is required")
	}

	signingRegion, endpoint, err := resolveFromConfig(context.Background(), cfg, service, sdkID)
	if err != nil {
		return nil, err
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		service:      service,
		apiVersion:   apiVersion,
		region:       signingRegion,
		endpoint:     endpoint,
		credentials:  cfg.Credentials,
		httpClient:   httpClient,
		signer:       v4.NewSigner(),
		nowFunc:      time.Now,
		maxBodyBytes: defaultMaxBodyBytes,
	}, nil
}

// Endpoint returns the resolved endpoint URL used by the client.
func (c *Client) Endpoint() string {
	return c.endpoint
}

// Region returns the normalized signing region used by the client.
func (c *Client) Region() string {
	return c.region
}

// Do executes a SigV4-signed POST request for the given AWS Query action and unmarshals
// the XML response into out (when non-nil).
func (c *Client) Do(ctx context.Context, action string, params url.Values, out any) error {
	if c.credentials == nil {
		return fmt.Errorf("AWS %s %s: missing credentials provider", c.service, action)
	}

	creds, err := c.credentials.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("AWS %s %s: failed to retrieve credentials: %w", c.service, action, err)
	}
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return fmt.Errorf("AWS %s %s: empty AWS credentials", c.service, action)
	}

	form := make(url.Values, len(params)+2)
	for k, vals := range params {
		form[k] = append([]string(nil), vals...)
	}
	form.Set("Action", action)
	form.Set("Version", c.apiVersion)

	bodyBytes := []byte(form.Encode())
	sum := sha256.Sum256(bodyBytes)
	payloadHash := hex.EncodeToString(sum[:])

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("AWS %s %s: failed to create request: %w", c.service, action, err)
	}
	if req.URL.Path == "" {
		req.URL.Path = "/"
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.ContentLength = int64(len(bodyBytes))

	if err := c.signer.SignHTTP(ctx, creds, req, payloadHash, c.service, c.region, c.nowFunc().UTC()); err != nil {
		return fmt.Errorf("AWS %s %s: failed to sign request: %w", c.service, action, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("AWS %s %s: request failed: %w", c.service, action, err)
	}
	defer resp.Body.Close()

	maxBytes := c.maxBodyBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBodyBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return fmt.Errorf("AWS %s %s: failed to read response body: %w", c.service, action, err)
	}
	if int64(len(body)) > maxBytes {
		return fmt.Errorf("AWS %s %s: response body exceeded maximum size of %d bytes", c.service, action, maxBytes)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseAPIError(c.service, action, resp.StatusCode, body)
	}

	if out != nil {
		if err := xml.Unmarshal(body, out); err != nil {
			return fmt.Errorf("AWS %s %s: failed to parse XML response: %w", c.service, action, err)
		}
	}
	return nil
}

type partition struct {
	id                 string
	regionRegexPattern string
	dnsSuffix          string
	dualStackDNSSuffix string
	supportsFIPS       bool
	supportsDualStack  bool
}

var (
	compilePartitionsOnce sync.Once
	compiledPartitions    []compiledPartition
	defaultAWSPartition   partition
)

type compiledPartition struct {
	partition
	regionRegex *regexp.Regexp
}

func initPartitions() {
	compilePartitionsOnce.Do(func() {
		compiledPartitions = make([]compiledPartition, 0, len(awsPartitions))
		for _, p := range awsPartitions {
			compiledPartitions = append(compiledPartitions, compiledPartition{
				partition:   p,
				regionRegex: regexp.MustCompile(p.regionRegexPattern),
			})
			if p.id == "aws" {
				defaultAWSPartition = p
			}
		}
	})
}

// partitionForRegion resolves the AWS partition matching region using the compiled
// RegionRegex table generated from aws-sdk-go-v2, falling back to the "aws" partition
// (matching aws-sdk-go-v2's awsrulesfn.GetPartition).
func partitionForRegion(region string) partition {
	initPartitions()
	for _, p := range compiledPartitions {
		if p.regionRegex.MatchString(region) {
			return p.partition
		}
	}
	return defaultAWSPartition
}

func resolveFromConfig(ctx context.Context, cfg aws.Config, service, sdkID string) (string, string, error) {
	region := cfg.Region

	useFIPS := false
	for _, src := range cfg.ConfigSources {
		if p, ok := src.(useFIPSEndpointProvider); ok {
			val, found, err := p.GetUseFIPSEndpoint(ctx)
			if err != nil {
				return "", "", fmt.Errorf("failed to resolve FIPS endpoint setting: %w", err)
			}
			if found {
				useFIPS = val == aws.FIPSEndpointStateEnabled
				break
			}
		}
	}

	// Support legacy pseudo-FIPS region identifiers (e.g. "fips-us-east-1", "us-east-1-fips", "us-east-fips-1")
	// matching aws-sdk-go-v2's MapFIPSRegion behavior.
	region, regionWasFIPS := stripFIPSRegion(region)
	if regionWasFIPS {
		useFIPS = true
	}

	useDualStack := false
	for _, src := range cfg.ConfigSources {
		if p, ok := src.(useDualStackEndpointProvider); ok {
			val, found, err := p.GetUseDualStackEndpoint(ctx)
			if err != nil {
				return "", "", fmt.Errorf("failed to resolve dual-stack endpoint setting: %w", err)
			}
			if found {
				useDualStack = val == aws.DualStackEndpointStateEnabled
				break
			}
		}
	}

	baseEndpoint := resolveBaseEndpoint(ctx, cfg, sdkID)

	endpoint, err := ResolveEndpoint(service, region, baseEndpoint, useFIPS, useDualStack)
	if err != nil {
		return "", "", err
	}
	return region, endpoint, nil
}

func resolveBaseEndpoint(ctx context.Context, cfg aws.Config, sdkID string) string {
	var baseEndpoint string
	if cfg.BaseEndpoint != nil {
		baseEndpoint = strings.TrimSpace(*cfg.BaseEndpoint)
	}

	for _, src := range cfg.ConfigSources {
		if p, ok := src.(ignoreConfiguredEndpointsProvider); ok {
			ignore, found, err := p.GetIgnoreConfiguredEndpoints(ctx)
			if err == nil && found && ignore {
				return ""
			}
			if err == nil && found {
				break
			}
		}
	}

	if sdkID == "" {
		return baseEndpoint
	}

	// Match aws-sdk-go-v2 precedence: if AWS_ENDPOINT_URL is set and AWS_ENDPOINT_URL_<SERVICE>
	// is not set, the global env var takes precedence over a profile's service-specific endpoint.
	serviceEnvKey := "AWS_ENDPOINT_URL_" + strings.ReplaceAll(strings.ToUpper(sdkID), " ", "_")
	_, hasGlobalEnv := os.LookupEnv("AWS_ENDPOINT_URL")
	_, hasServiceEnv := os.LookupEnv(serviceEnvKey)
	if hasGlobalEnv && !hasServiceEnv {
		return baseEndpoint
	}

	for _, src := range cfg.ConfigSources {
		if p, ok := src.(serviceBaseEndpointProvider); ok {
			val, found, err := p.GetServiceBaseEndpoint(ctx, sdkID)
			if err == nil && found && strings.TrimSpace(val) != "" {
				return strings.TrimSpace(val)
			}
		}
	}

	return baseEndpoint
}

func stripFIPSRegion(region string) (string, bool) {
	const (
		fipsInfix  = "-fips-"
		fipsPrefix = "fips-"
		fipsSuffix = "-fips"
	)
	if !strings.Contains(region, fipsInfix) &&
		!strings.HasPrefix(region, fipsPrefix) &&
		!strings.HasSuffix(region, fipsSuffix) {
		return region, false
	}
	cleaned := strings.ReplaceAll(region, fipsInfix, "-")
	cleaned = strings.TrimPrefix(cleaned, fipsPrefix)
	cleaned = strings.TrimSuffix(cleaned, fipsSuffix)
	return cleaned, true
}

// ResolveEndpoint computes the AWS endpoint URL for service and region, honoring custom
// baseEndpoint, FIPS, dual-stack, and AWS partition DNS suffixes (matching aws-sdk-go-v2
// endpoint rules v2 for EC2 and RDS).
func ResolveEndpoint(service, region, baseEndpoint string, useFIPS, useDualStack bool) (string, error) {
	if service == "" {
		return "", errors.New("AWS service name is required")
	}
	if baseEndpoint != "" {
		if useFIPS {
			return "", errors.New("invalid configuration: FIPS and custom endpoint are not supported")
		}
		if useDualStack {
			return "", errors.New("invalid configuration: dual-stack and custom endpoint are not supported")
		}
		u, err := url.Parse(baseEndpoint)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return "", fmt.Errorf("invalid custom AWS endpoint URL %q", baseEndpoint)
		}
		// Match aws-sdk-go-v2 ResolveEndpointV2 behavior: only scheme, host, and path
		// are carried from BaseEndpoint (query parameters and fragments are stripped).
		u.RawQuery = ""
		u.Fragment = ""
		return u.String(), nil
	}

	if region == "" {
		return "", errors.New("missing AWS region")
	}
	if !isValidRegion(region) {
		return "", fmt.Errorf("invalid AWS region %q", region)
	}

	part := partitionForRegion(region)

	if useFIPS && useDualStack && (!part.supportsFIPS || !part.supportsDualStack) {
		return "", fmt.Errorf("FIPS and dual-stack are enabled, but partition %q does not support one or both", part.id)
	}
	if useFIPS && !part.supportsFIPS {
		return "", fmt.Errorf("FIPS is enabled, but partition %q does not support FIPS", part.id)
	}
	if useDualStack && !part.supportsDualStack {
		return "", fmt.Errorf("dual-stack is enabled, but partition %q does not support dual-stack", part.id)
	}

	hostPrefix := service
	if useFIPS {
		// In aws-us-gov, standard (non-dualstack) EC2 and RDS endpoints are already FIPS-compliant
		// and do not use a "-fips" subdomain (matching aws-sdk-go-v2 endpoint rules).
		if !(part.id == "aws-us-gov" && !useDualStack) {
			hostPrefix = service + "-fips"
		}
	}

	suffix := part.dnsSuffix
	if useDualStack {
		suffix = part.dualStackDNSSuffix
	}

	return fmt.Sprintf("https://%s.%s.%s", hostPrefix, region, suffix), nil
}

// isValidRegion matches aws-sdk-go-v2's rulesfn.IsValidHostLabel(region, true),
// allowing one or more dot-separated DNS host labels.
func isValidRegion(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if !isValidHostLabel(label) {
			return false
		}
	}
	return true
}

func isValidHostLabel(s string) bool {
	if len(s) == 0 || len(s) > 63 {
		return false
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isAlphaNum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !isAlphaNum && c != '-' {
			return false
		}
	}
	return true
}

type ec2ErrorResponse struct {
	XMLName xml.Name `xml:"Response"`
	Errors  []struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	} `xml:"Errors>Error"`
	RequestID string `xml:"RequestID"`
}

type awsQueryErrorResponse struct {
	XMLName xml.Name `xml:"ErrorResponse"`
	Error   struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	} `xml:"Error"`
	RequestID string `xml:"RequestId"`
}

func parseAPIError(service, action string, statusCode int, body []byte) error {
	var ec2Err ec2ErrorResponse
	if err := xml.Unmarshal(body, &ec2Err); err == nil && len(ec2Err.Errors) > 0 {
		return &APIError{
			Service:    service,
			Action:     action,
			StatusCode: statusCode,
			Code:       ec2Err.Errors[0].Code,
			Message:    ec2Err.Errors[0].Message,
			RequestID:  ec2Err.RequestID,
		}
	}

	var queryErr awsQueryErrorResponse
	if err := xml.Unmarshal(body, &queryErr); err == nil && (queryErr.Error.Code != "" || queryErr.Error.Message != "") {
		return &APIError{
			Service:    service,
			Action:     action,
			StatusCode: statusCode,
			Code:       queryErr.Error.Code,
			Message:    queryErr.Error.Message,
			RequestID:  queryErr.RequestID,
		}
	}

	snippet := strings.TrimSpace(string(body))
	if len(snippet) > maxErrorSnippetBytes {
		snippet = snippet[:maxErrorSnippetBytes] + "..."
	}
	return &APIError{
		Service:    service,
		Action:     action,
		StatusCode: statusCode,
		Message:    snippet,
	}
}
