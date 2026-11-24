// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-Present Datadog, Inc.

package localscope

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

)

// ProviderName is the DelegatedTokenConfig.Provider value for this package.
const ProviderName = "oidc"

const (
	audiencePrefix  = "datadog/"
	proofPrefix     = "oidc-"
	maxTokenSize    = 64 * 1024
	maxResponseSize = 1 << 20
)

// Option configures a Provider.
type Option func(*Provider) error

// WithTokenFile reads the workload token from path on every exchange, so rotated files are picked up.
func WithTokenFile(path string) Option {
	return func(provider *Provider) error {
		if path == "" {
			return errors.New("workload token file path must not be empty")
		}
		provider.tokenFile = path
		return nil
	}
}

// WithTokenEnv reads the workload token from the named environment variable on every exchange.
func WithTokenEnv(name string) Option {
	return func(provider *Provider) error {
		if name == "" {
			return errors.New("workload token environment variable name must not be empty")
		}
		provider.tokenEnv = name
		return nil
	}
}

// Provider implements datadog.DelegatedTokenProvider for custom OIDC workload identity tokens.
type Provider struct {
	tokenFile  string
	tokenEnv   string
	now        func() time.Time
}


// New creates a custom OIDC delegated-token provider; exactly one token source is required.
func New(options ...Option) (*Provider, error) {
	provider := &Provider{now: time.Now}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(provider); err != nil {
			return nil, err
		}
	}
	if (provider.tokenFile == "") == (provider.tokenEnv == "") {
		return nil, errors.New("exactly one of WithTokenFile or WithTokenEnv is required")
	}
	return provider, nil
}

// Proof reads the workload token, refuses it unless it is Datadog-scoped, and returns the Delegated proof.
func (provider *Provider) Proof(orgUUID string) (string, error) {
	if orgUUID == "" {
		return "", errors.New("missing org UUID in delegated token configuration")
	}
	token, err := provider.readToken()
	if err != nil {
		return "", err
	}
	// The token must never reach Datadog unless it is addressed only to Datadog.
	if err := CheckDatadogScoped(token, orgUUID, provider.now()); err != nil {
		return "", err
	}
	return proofPrefix + token, nil
}

func (provider *Provider) readToken() (string, error) {
	var raw string
	if provider.tokenFile != "" {
		content, err := readTokenFile(provider.tokenFile)
		if err != nil {
			return "", err
		}
		raw = string(content)
	} else {
		value, ok := os.LookupEnv(provider.tokenEnv)
		if !ok {
			return "", fmt.Errorf("workload token environment variable %s is not set", provider.tokenEnv)
		}
		raw = value
	}

	token := strings.TrimSpace(raw)
	if token == "" {
		return "", errors.New("workload token is empty")
	}
	if len(token) > maxTokenSize {
		return "", fmt.Errorf("workload token exceeds %d bytes", maxTokenSize)
	}
	for i := 0; i < len(token); i++ {
		if token[i] < 0x21 || token[i] > 0x7e {
			return "", errors.New("workload token contains whitespace or non-printable characters")
		}
	}
	return token, nil
}

// readTokenFile refuses non-regular files before and after opening, because opening a FIFO blocks.
func readTokenFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("opening workload token file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("workload token file is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening workload token file: %w", err)
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("workload token file is not a regular file")
	}
	content, err := io.ReadAll(io.LimitReader(file, maxTokenSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading workload token file: %w", err)
	}
	return content, nil
}

// TokenScopeError reports why a workload token was refused before contacting Datadog.
type TokenScopeError struct {
	Reason string
}

func (err *TokenScopeError) Error() string {
	return "workload token is not scoped to Datadog: " + err.Reason
}

type audience []string

func (value *audience) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*value = audience{single}
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return errors.New("aud is neither a string nor a list of strings")
	}
	*value = list
	return nil
}

type scopeClaims struct {
	Audience audience
	Expiry   *float64
	OrgUUID  string
}

// CheckDatadogScoped requires a single datadog/{orgUUID} audience; it does not verify the signature.
func CheckDatadogScoped(token, orgUUID string, now time.Time) error {
	claims, err := decodeClaims(token)
	if err != nil {
		return &TokenScopeError{Reason: err.Error()}
	}
	want := audiencePrefix + orgUUID
	// Reasons omit claim and org values because they reach logs and support flares.
	switch {
	case len(claims.Audience) == 0:
		return &TokenScopeError{Reason: "no aud claim"}
	case len(claims.Audience) > 1:
		return &TokenScopeError{Reason: fmt.Sprintf("aud has %d values; only a single Datadog audience for the configured org is accepted", len(claims.Audience))}
	case claims.Audience[0] != want && strings.HasPrefix(claims.Audience[0], audiencePrefix):
		return &TokenScopeError{Reason: "aud is a datadog/ audience that does not match the configured org UUID"}
	case claims.Audience[0] != want:
		return &TokenScopeError{Reason: "aud is not a Datadog audience"}
	}
	if claims.OrgUUID != "" && claims.OrgUUID != orgUUID {
		return &TokenScopeError{Reason: "dd_org_uuid does not match the configured org UUID"}
	}
	if claims.Expiry == nil {
		return &TokenScopeError{Reason: "no exp claim"}
	}
	if expiry := time.Unix(int64(*claims.Expiry), 0); !now.Before(expiry) {
		return &TokenScopeError{Reason: "token expired at " + expiry.UTC().Format(time.RFC3339)}
	}
	return nil
}

func decodeClaims(token string) (*scopeClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("JWT payload is not base64url")
	}
	// Claim names are case-sensitive; struct decoding would let "AUD" override "aud".
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, errors.New("JWT payload is not a JSON object")
	}
	var claims scopeClaims
	if value, ok := raw["aud"]; ok {
		if err := json.Unmarshal(value, &claims.Audience); err != nil {
			return nil, err
		}
	}
	if value, ok := raw["exp"]; ok {
		var expiry float64
		if err := json.Unmarshal(value, &expiry); err != nil {
			return nil, errors.New("exp is not a number")
		}
		claims.Expiry = &expiry
	}
	if value, ok := raw["dd_org_uuid"]; ok {
		if err := json.Unmarshal(value, &claims.OrgUUID); err != nil {
			return nil, errors.New("dd_org_uuid is not a string")
		}
	}
	return &claims, nil
}

