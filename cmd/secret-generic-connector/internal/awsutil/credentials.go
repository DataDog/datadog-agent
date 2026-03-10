// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package awsutil

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Credentials holds AWS access credentials.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

// AWSConfig holds resolved AWS credentials and region.
type AWSConfig struct {
	Credentials Credentials
	Region      string
}

// SessionConfig maps to the user-facing connector configuration.
type SessionConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Profile         string
	RoleArn         string
	ExternalID      string
}

// ResolveConfig resolves AWS credentials and region from the given session
// config, falling back through environment variables, shared credential files,
// ECS container credentials, and EC2 IMDS.
func ResolveConfig(ctx context.Context, cfg SessionConfig) (*AWSConfig, error) {
	creds, err := resolveCreds(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve AWS credentials: %w", err)
	}

	region := resolveRegion(cfg)

	result := &AWSConfig{Credentials: *creds, Region: region}

	// If a role ARN is specified, assume that role using STS.
	if cfg.RoleArn != "" {
		assumed, err := assumeRole(ctx, result, cfg.RoleArn, cfg.ExternalID)
		if err != nil {
			return nil, fmt.Errorf("failed to assume role %s: %w", cfg.RoleArn, err)
		}
		result.Credentials = *assumed
	}

	return result, nil
}

// resolveCreds tries each source in order until credentials are found.
func resolveCreds(ctx context.Context, cfg SessionConfig) (*Credentials, error) {
	// 1. Explicit static credentials from config.
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		return &Credentials{
			AccessKeyID:     cfg.AccessKeyID,
			SecretAccessKey: cfg.SecretAccessKey,
		}, nil
	}

	// 2. Environment variables.
	if id, secret := os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"); id != "" && secret != "" {
		return &Credentials{
			AccessKeyID:     id,
			SecretAccessKey: secret,
			SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
		}, nil
	}

	// 3. Shared credentials file (~/.aws/credentials).
	profile := cfg.Profile
	if profile == "" {
		profile = os.Getenv("AWS_PROFILE")
	}
	if profile == "" {
		profile = "default"
	}
	if creds := readSharedCredentials(profile); creds != nil {
		return creds, nil
	}

	// 4. ECS container credentials.
	if creds, err := ecsContainerCredentials(ctx); err == nil && creds != nil {
		return creds, nil
	}

	// 5. EC2 IMDS v2.
	if creds, err := ec2IMDSCredentials(ctx); err == nil && creds != nil {
		return creds, nil
	}

	return nil, errors.New("no AWS credentials found (checked config, env, shared credentials, ECS, IMDS)")
}

func resolveRegion(cfg SessionConfig) string {
	if cfg.Region != "" {
		return cfg.Region
	}
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		return r
	}

	profile := cfg.Profile
	if profile == "" {
		profile = os.Getenv("AWS_PROFILE")
	}
	if profile == "" {
		profile = "default"
	}
	if r := readSharedConfigRegion(profile); r != "" {
		return r
	}

	return ""
}

// --- Shared credentials file ---

func awsDir() string {
	if d := os.Getenv("AWS_SHARED_CREDENTIALS_FILE"); d != "" {
		return filepath.Dir(d)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".aws")
}

func credentialsFilePath() string {
	if f := os.Getenv("AWS_SHARED_CREDENTIALS_FILE"); f != "" {
		return f
	}
	dir := awsDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "credentials")
}

func configFilePath() string {
	if f := os.Getenv("AWS_CONFIG_FILE"); f != "" {
		return f
	}
	dir := awsDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "config")
}

func readSharedCredentials(profile string) *Credentials {
	path := credentialsFilePath()
	if path == "" {
		return nil
	}
	section := readINISection(path, profile)
	if section == nil {
		return nil
	}
	id := section["aws_access_key_id"]
	secret := section["aws_secret_access_key"]
	if id == "" || secret == "" {
		return nil
	}
	return &Credentials{
		AccessKeyID:     id,
		SecretAccessKey: secret,
		SessionToken:    section["aws_session_token"],
	}
}

func readSharedConfigRegion(profile string) string {
	path := configFilePath()
	if path == "" {
		return ""
	}
	// In ~/.aws/config, non-default profiles use "profile <name>" as section header.
	sectionName := profile
	if profile != "default" {
		sectionName = "profile " + profile
	}
	section := readINISection(path, sectionName)
	if section == nil {
		return ""
	}
	return section["region"]
}

// readINISection reads a section from an INI file. Returns nil if not found.
func readINISection(path, section string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	result := make(map[string]string)
	inSection := false
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.TrimSpace(line[1 : len(line)-1])
			if inSection {
				// We've left our section.
				return result
			}
			inSection = (name == section)
			continue
		}
		if inSection {
			if idx := strings.IndexByte(line, '='); idx > 0 {
				key := strings.TrimSpace(line[:idx])
				val := strings.TrimSpace(line[idx+1:])
				result[key] = val
			}
		}
	}
	if inSection {
		return result
	}
	return nil
}

// --- ECS container credentials ---

func ecsContainerCredentials(ctx context.Context) (*Credentials, error) {
	relURI := os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI")
	fullURI := os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI")

	var endpoint string
	var authToken string
	switch {
	case relURI != "":
		endpoint = "http://169.254.170.2" + relURI
	case fullURI != "":
		endpoint = fullURI
		authToken = os.Getenv("AWS_CONTAINER_AUTHORIZATION_TOKEN")
	default:
		return nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	if authToken != "" {
		req.Header.Set("Authorization", authToken)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parseIMDSCredentialResponse(resp.Body)
}

// --- EC2 IMDS v2 ---

const imdsEndpoint = "http://169.254.169.254"

func ec2IMDSCredentials(ctx context.Context) (*Credentials, error) {
	client := &http.Client{Timeout: 2 * time.Second}

	// Get IMDSv2 token.
	tokenReq, err := http.NewRequestWithContext(ctx, "PUT", imdsEndpoint+"/latest/api/token", nil)
	if err != nil {
		return nil, err
	}
	tokenReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")

	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return nil, err
	}
	tokenBytes, _ := io.ReadAll(tokenResp.Body)
	tokenResp.Body.Close()
	if tokenResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IMDS token request returned %d", tokenResp.StatusCode)
	}
	token := string(tokenBytes)

	// Get IAM role name.
	roleReq, err := http.NewRequestWithContext(ctx, "GET", imdsEndpoint+"/latest/meta-data/iam/security-credentials/", nil)
	if err != nil {
		return nil, err
	}
	roleReq.Header.Set("X-aws-ec2-metadata-token", token)
	roleResp, err := client.Do(roleReq)
	if err != nil {
		return nil, err
	}
	roleBytes, _ := io.ReadAll(roleResp.Body)
	roleResp.Body.Close()
	if roleResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IMDS role request returned %d", roleResp.StatusCode)
	}
	role := strings.TrimSpace(string(roleBytes))
	if role == "" {
		return nil, errors.New("no IAM role found in IMDS")
	}

	// Get credentials for role.
	credReq, err := http.NewRequestWithContext(ctx, "GET", imdsEndpoint+"/latest/meta-data/iam/security-credentials/"+role, nil)
	if err != nil {
		return nil, err
	}
	credReq.Header.Set("X-aws-ec2-metadata-token", token)
	credResp, err := client.Do(credReq)
	if err != nil {
		return nil, err
	}
	defer credResp.Body.Close()
	if credResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IMDS credentials request returned %d", credResp.StatusCode)
	}

	return parseIMDSCredentialResponse(credResp.Body)
}

type imdsCredentialResponse struct {
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	Token           string `json:"Token"`
}

func parseIMDSCredentialResponse(body io.Reader) (*Credentials, error) {
	var resp imdsCredentialResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, err
	}
	if resp.AccessKeyID == "" || resp.SecretAccessKey == "" {
		return nil, errors.New("empty credentials from IMDS")
	}
	return &Credentials{
		AccessKeyID:     resp.AccessKeyID,
		SecretAccessKey: resp.SecretAccessKey,
		SessionToken:    resp.Token,
	}, nil
}

// --- STS AssumeRole ---

// assumeRoleResponse models the STS AssumeRole XML response.
type assumeRoleResponse struct {
	XMLName xml.Name `xml:"AssumeRoleResponse"`
	Result  struct {
		Credentials struct {
			AccessKeyID     string `xml:"AccessKeyId"`
			SecretAccessKey string `xml:"SecretAccessKey"`
			SessionToken    string `xml:"SessionToken"`
		} `xml:"Credentials"`
	} `xml:"AssumeRoleResult"`
}

func assumeRole(ctx context.Context, cfg *AWSConfig, roleArn, externalID string) (*Credentials, error) {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	endpoint := stsEndpoint(region)

	params := url.Values{
		"Action":          {"AssumeRole"},
		"Version":         {"2011-06-15"},
		"RoleArn":         {roleArn},
		"RoleSessionName": {"secret-generic-connector"},
	}
	if externalID != "" {
		params.Set("ExternalId", externalID)
	}

	body := []byte(params.Encode())
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	SignRequest(req, cfg.Credentials, region, "sts", body)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("STS AssumeRole returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result assumeRoleResponse
	if err := xml.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse STS response: %w", err)
	}

	return &Credentials{
		AccessKeyID:     result.Result.Credentials.AccessKeyID,
		SecretAccessKey: result.Result.Credentials.SecretAccessKey,
		SessionToken:    result.Result.Credentials.SessionToken,
	}, nil
}

func stsEndpoint(region string) string {
	// Use regional STS endpoints.
	if region == "" {
		return "https://sts.amazonaws.com/"
	}
	if isChina(region) {
		return fmt.Sprintf("https://sts.%s.amazonaws.com.cn/", region)
	}
	if isGov(region) {
		return fmt.Sprintf("https://sts.%s.amazonaws.com/", region)
	}
	return fmt.Sprintf("https://sts.%s.amazonaws.com/", region)
}

func isChina(region string) bool  { return strings.HasPrefix(region, "cn-") }
func isGov(region string) bool    { return strings.HasPrefix(region, "us-gov-") }

// ServiceEndpoint returns the endpoint for an AWS service in the given region.
func ServiceEndpoint(service, region string) string {
	if isChina(region) {
		return fmt.Sprintf("https://%s.%s.amazonaws.com.cn/", service, region)
	}
	return fmt.Sprintf("https://%s.%s.amazonaws.com/", service, region)
}

// UserHomeDir returns the user's home directory in a cross-platform way.
// This replaces the dependency on github.com/mitchellh/go-homedir.
func userHomeDir() string {
	if runtime.GOOS == "windows" {
		if home := os.Getenv("USERPROFILE"); home != "" {
			return home
		}
	}
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return home
}
