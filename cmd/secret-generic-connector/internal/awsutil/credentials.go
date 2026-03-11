// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package awsutil

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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

	region := resolveRegion(ctx, cfg)

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

	// 3. Web Identity Token (IRSA / EKS Pod Identity).
	if tokenFile := os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"); tokenFile != "" {
		if roleArn := os.Getenv("AWS_ROLE_ARN"); roleArn != "" {
			sessionName := os.Getenv("AWS_ROLE_SESSION_NAME")
			if sessionName == "" {
				sessionName = "secret-generic-connector"
			}
			region := resolveRegionEnvOnly(cfg)
			if creds, err := assumeRoleWithWebIdentity(ctx, region, roleArn, sessionName, tokenFile); err == nil {
				return creds, nil
			}
		}
	}

	// 4. Shared credentials file (~/.aws/credentials).
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

	// 5. credential_process from config file.
	if creds, err := credentialProcessCredentials(ctx, profile); err == nil && creds != nil {
		return creds, nil
	}

	// 6. SSO cached credentials.
	if creds, err := ssoCredentials(ctx, profile); err == nil && creds != nil {
		return creds, nil
	}

	// 7. ECS container credentials.
	if creds, err := ecsContainerCredentials(ctx); err == nil && creds != nil {
		return creds, nil
	}

	// 8. EC2 IMDS v2.
	if creds, err := ec2IMDSCredentials(ctx); err == nil && creds != nil {
		return creds, nil
	}

	return nil, errors.New("no AWS credentials found (checked config, env, web identity, shared credentials, credential_process, SSO, ECS, IMDS)")
}

// resolveRegionEnvOnly resolves region from config and env vars only (no network calls).
// Used during credential resolution when we need a region hint but can't do IMDS yet.
func resolveRegionEnvOnly(cfg SessionConfig) string {
	if cfg.Region != "" {
		return cfg.Region
	}
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		return r
	}
	return ""
}

func resolveRegion(ctx context.Context, cfg SessionConfig) string {
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

	// Fall back to IMDS for region discovery on EC2.
	if r, err := imdsRegion(ctx); err == nil && r != "" {
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

// imdsBaseURL can be overridden in tests.
var imdsBaseURL = "http://169.254.169.254"

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

// --- IMDS region ---

func imdsRegion(ctx context.Context) (string, error) {
	base := imdsBaseURL
	client := &http.Client{Timeout: 2 * time.Second}

	// Get IMDSv2 token.
	tokenReq, err := http.NewRequestWithContext(ctx, "PUT", base+"/latest/api/token", nil)
	if err != nil {
		return "", err
	}
	tokenReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")

	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return "", err
	}
	tokenBytes, _ := io.ReadAll(tokenResp.Body)
	tokenResp.Body.Close()
	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IMDS token request returned %d", tokenResp.StatusCode)
	}
	token := string(tokenBytes)

	regionReq, err := http.NewRequestWithContext(ctx, "GET", base+"/latest/meta-data/placement/region", nil)
	if err != nil {
		return "", err
	}
	regionReq.Header.Set("X-aws-ec2-metadata-token", token)
	regionResp, err := client.Do(regionReq)
	if err != nil {
		return "", err
	}
	defer regionResp.Body.Close()
	if regionResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IMDS region request returned %d", regionResp.StatusCode)
	}
	regionBytes, err := io.ReadAll(regionResp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(regionBytes)), nil
}

// --- Web Identity Token (IRSA) ---

// stsWebIdentityEndpoint can be overridden in tests.
var stsWebIdentityEndpoint = ""

// assumeRoleWithWebIdentityResponse models the STS AssumeRoleWithWebIdentity XML response.
type assumeRoleWithWebIdentityResponse struct {
	XMLName xml.Name `xml:"AssumeRoleWithWebIdentityResponse"`
	Result  struct {
		Credentials struct {
			AccessKeyID     string `xml:"AccessKeyId"`
			SecretAccessKey string `xml:"SecretAccessKey"`
			SessionToken    string `xml:"SessionToken"`
		} `xml:"Credentials"`
	} `xml:"AssumeRoleWithWebIdentityResult"`
}

func assumeRoleWithWebIdentity(ctx context.Context, region, roleArn, sessionName, tokenFile string) (*Credentials, error) {
	tokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read web identity token file: %w", err)
	}
	token := strings.TrimSpace(string(tokenBytes))

	endpoint := stsWebIdentityEndpoint
	if endpoint == "" {
		endpoint = stsEndpoint(region)
	}

	params := url.Values{
		"Action":           {"AssumeRoleWithWebIdentity"},
		"Version":          {"2011-06-15"},
		"RoleArn":          {roleArn},
		"RoleSessionName":  {sessionName},
		"WebIdentityToken": {token},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// This call is unsigned — the JWT is the proof of identity.

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
		return nil, fmt.Errorf("STS AssumeRoleWithWebIdentity returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result assumeRoleWithWebIdentityResponse
	if err := xml.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse STS response: %w", err)
	}

	return &Credentials{
		AccessKeyID:     result.Result.Credentials.AccessKeyID,
		SecretAccessKey: result.Result.Credentials.SecretAccessKey,
		SessionToken:    result.Result.Credentials.SessionToken,
	}, nil
}

// --- credential_process ---

// execCommand can be overridden in tests.
var execCommand = exec.CommandContext

type credentialProcessOutput struct {
	Version         int    `json:"Version"`
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	SessionToken    string `json:"SessionToken"`
}

func credentialProcessCredentials(ctx context.Context, profile string) (*Credentials, error) {
	path := configFilePath()
	if path == "" {
		return nil, nil
	}

	sectionName := profile
	if profile != "default" {
		sectionName = "profile " + profile
	}
	section := readINISection(path, sectionName)
	if section == nil {
		return nil, nil
	}
	command := section["credential_process"]
	if command == "" {
		return nil, nil
	}

	cmd := execCommand(ctx, "sh", "-c", command)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("credential_process failed: %w", err)
	}

	var result credentialProcessOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("credential_process output is not valid JSON: %w", err)
	}
	if result.Version != 1 {
		return nil, fmt.Errorf("credential_process: unsupported version %d", result.Version)
	}
	if result.AccessKeyID == "" || result.SecretAccessKey == "" {
		return nil, errors.New("credential_process returned empty credentials")
	}

	return &Credentials{
		AccessKeyID:     result.AccessKeyID,
		SecretAccessKey: result.SecretAccessKey,
		SessionToken:    result.SessionToken,
	}, nil
}

// --- SSO credentials ---

// ssoGetRoleEndpoint can be overridden in tests.
var ssoGetRoleEndpoint = ""

type ssoCachedToken struct {
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

type ssoRoleCredentialsResponse struct {
	RoleCredentials struct {
		AccessKeyID     string `json:"accessKeyId"`
		SecretAccessKey string `json:"secretAccessKey"`
		SessionToken    string `json:"sessionToken"`
	} `json:"roleCredentials"`
}

func ssoCredentials(ctx context.Context, profile string) (*Credentials, error) {
	path := configFilePath()
	if path == "" {
		return nil, nil
	}

	sectionName := profile
	if profile != "default" {
		sectionName = "profile " + profile
	}
	section := readINISection(path, sectionName)
	if section == nil {
		return nil, nil
	}

	startURL := section["sso_start_url"]
	ssoRegion := section["sso_region"]
	accountID := section["sso_account_id"]
	roleName := section["sso_role_name"]
	if startURL == "" || ssoRegion == "" || accountID == "" || roleName == "" {
		return nil, nil
	}

	// Compute cache key: SHA1 of sso_start_url.
	h := sha1.New() //nolint:gosec
	h.Write([]byte(startURL))
	cacheKey := hex.EncodeToString(h.Sum(nil))

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	cachePath := filepath.Join(home, ".aws", "sso", "cache", cacheKey+".json")

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("SSO cache file not found: %w", err)
	}

	var cached ssoCachedToken
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("failed to parse SSO cache: %w", err)
	}
	if cached.AccessToken == "" {
		return nil, errors.New("SSO cache has empty access token")
	}

	endpoint := ssoGetRoleEndpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://portal.sso.%s.amazonaws.com", ssoRegion)
	}

	reqURL := fmt.Sprintf("%s/federation/credentials?role_name=%s&account_id=%s",
		endpoint, url.QueryEscape(roleName), url.QueryEscape(accountID))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-amz-sso_bearer_token", cached.AccessToken)

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
		return nil, fmt.Errorf("SSO GetRoleCredentials returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result ssoRoleCredentialsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse SSO response: %w", err)
	}

	if result.RoleCredentials.AccessKeyID == "" || result.RoleCredentials.SecretAccessKey == "" {
		return nil, errors.New("SSO returned empty credentials")
	}

	return &Credentials{
		AccessKeyID:     result.RoleCredentials.AccessKeyID,
		SecretAccessKey: result.RoleCredentials.SecretAccessKey,
		SessionToken:    result.RoleCredentials.SessionToken,
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

func isChina(region string) bool { return strings.HasPrefix(region, "cn-") }
func isGov(region string) bool   { return strings.HasPrefix(region, "us-gov-") }

// ServiceEndpoint returns the endpoint for an AWS service in the given region.
func ServiceEndpoint(service, region string) string {
	if isChina(region) {
		return fmt.Sprintf("https://%s.%s.amazonaws.com.cn/", service, region)
	}
	return fmt.Sprintf("https://%s.%s.amazonaws.com/", service, region)
}
