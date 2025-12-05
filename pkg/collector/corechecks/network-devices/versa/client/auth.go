// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type (
	// authMethod indicates the authentication method options for
	// the Versa integration
	authMethod string

	// AuthConfig encapsulates authentication configuration for the Versa client
	AuthConfig struct {
		Method       string
		Username     string
		Password     string
		ClientID     string
		ClientSecret string
	}

	// OAuthRequest encapsulates data for performing OAuth
	OAuthRequest struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Username     string `json:"username"`
		Password     string `json:"password"`
		GrantType    string `json:"grant_type"`
	}

	// OAuthResponse encapsulates Versa OAuth responses
	OAuthResponse struct {
		AccessToken  string `json:"access_token"`
		IssuedAt     string `json:"issued_at"`
		ExpiresIn    string `json:"expires_in"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
	}
)

const (
	// authMethodBasic specifies that Basic Auth should be used
	// for Director API calls
	authMethodBasic authMethod = "basic"
	// authMethodOAuth specifies that OAuth should be used for
	// Director API calls
	authMethodOAuth authMethod = "oauth"
)

// Parse takes a string and attempts to parse it into a valid authMethod
func (a *authMethod) Parse(authString string) error {
	method := authMethod(strings.ToLower(authString))
	switch method {
	case authMethodBasic, authMethodOAuth:
		*a = method
		return nil
	default:
		return fmt.Errorf("invalid auth method %q, valid auth methods: %q, %q", authString, authMethodBasic, authMethodOAuth)
	}
}

// processAuthConfig validates and parses the authentication configuration
func processAuthConfig(config AuthConfig) (authMethod, error) {
	if config.Username == "" {
		return "", errors.New("username is required")
	}
	if config.Password == "" {
		return "", errors.New("password is required")
	}

	// Parse and validate the auth method (if provided)
	authMethod := authMethodBasic // default
	if config.Method != "" {
		err := authMethod.Parse(config.Method)
		if err != nil {
			return "", fmt.Errorf("invalid auth_method: %w", err)
		}
	}

	// Validate OAuth specific requirements
	if authMethod == authMethodOAuth {
		if config.ClientID == "" || config.ClientSecret == "" {
			return "", errors.New("client_id and client_secret are required for OAuth authentication")
		}
	}

	return authMethod, nil
}

// loginSession logs in to the Versa Director API using session-based authentication
// This allows Analytics calls to be proxied through the director
func (client *Client) loginSession() error {
	authPayload := url.Values{}
	authPayload.Set("j_username", client.username)
	authPayload.Set("j_password", client.password)

	// Run GET request to get session cookie and CSRF token
	err := client.runGetCSRFToken()
	if err != nil {
		return fmt.Errorf("failed to get CSRF token: %w", err)
	}

	// now we can actually login and get a session cookie
	err = client.runJSpringSecurityCheck(&authPayload)
	if err != nil {
		return fmt.Errorf("failed to run j_spring_security_check to get session token: %w", err)
	}

	// Request to /versa/analytics/login to obtain Analytics CSRF prevention token
	analyticsPayload := url.Values{}
	analyticsPayload.Set("endpoint", client.analyticsEndpoint)

	err = client.runAnalyticsLogin(&analyticsPayload)
	if err != nil {
		return fmt.Errorf("failed to perform analytics login: %w", err)
	}

	return nil
}

// loginOAuth logs in to the Director API using OAuth
func (client *Client) loginOAuth() error {
	reqBody, err := json.Marshal(OAuthRequest{
		ClientID:     client.clientID,
		ClientSecret: client.clientSecret,
		Username:     client.username,
		Password:     client.password,
		GrantType:    "password",
	})
	if err != nil {
		return fmt.Errorf("failed to marshal oauth request body: %v", err)
	}

	// Request to /auth/token to perform OAuth authentication
	req, err := client.newRequest("POST", "/auth/token", bytes.NewReader(reqBody), false)
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	// Execute the request
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("OAuth request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read OAuth response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("OAuth authentication failed, status code: %v: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse JSON response
	var oauthResp OAuthResponse
	err = json.Unmarshal(bodyBytes, &oauthResp)
	if err != nil {
		// If JSON parsing fails, return the raw response for debugging
		return fmt.Errorf("failed to parse OAuth response as JSON: %v, response: %s", err, string(bodyBytes))
	}

	// Set director token and expiration on the client
	client.directorToken = oauthResp.AccessToken

	// Handle expiry
	expiresInSeconds, err := strconv.ParseInt(oauthResp.ExpiresIn, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse expires_in as integer: %v, value: %s", err, oauthResp.ExpiresIn)
	}
	expirationDuration := time.Duration(expiresInSeconds) * time.Second
	client.directorTokenExpiry = timeNow().Add(expirationDuration)

	log.Trace("OAuth authentication successful")
	return nil
}

// authenticateDirector handles Director API authentication (OAuth or Basic - Basic doesn't need pre-auth)
func (client *Client) authenticateDirector() error {
	switch client.authMethod {
	case authMethodBasic:
		return nil
	case authMethodOAuth:
		now := timeNow()
		client.authenticationMutex.Lock()
		defer client.authenticationMutex.Unlock()

		if client.directorToken == "" || client.directorTokenExpiry.Before(now) {
			return client.loginOAuth()
		}
		return nil
	default:
		return fmt.Errorf("unsupported authentication method: %s", client.authMethod)
	}
}

// authenticateSession handles session authentication for Analytics endpoints
func (client *Client) authenticateSession() error {
	now := timeNow()
	client.authenticationMutex.Lock()
	defer client.authenticationMutex.Unlock()

	if client.sessionToken == "" || client.sessionTokenExpiry.Before(now) {
		return client.loginSession()
	}
	return nil
}

// clearAuth clears both director and session auth state
func (client *Client) clearAuth() {
	client.authenticationMutex.Lock()
	client.directorToken = ""
	client.sessionToken = ""
	client.authenticationMutex.Unlock()
}

// isAuthenticated determine if a request was successful from the headers
// Vera can return HTTP 200 when auth is invalid, with the HTML login page
// API calls returns application/json when successful. We assume receiving HTML means we're unauthenticated.
func isAuthenticated(headers http.Header) bool {
	content := headers.Get("content-type")
	return !strings.HasPrefix(content, "text/html")
}

func (client *Client) runGetCSRFToken() error {
	req, err := client.newRequest("GET", "/versa/analytics/auth/user", nil, true)
	if err != nil {
		return err
	}

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	endpointURL, err := url.Parse(client.directorEndpoint + "/versa")
	if err != nil {
		return fmt.Errorf("url parsing failed: %w", err)
	}
	cookies := client.httpClient.Jar.Cookies(endpointURL)
	for _, cookie := range cookies {
		if cookie.Name == "VD-CSRF-TOKEN" {
			client.sessionToken = cookie.Value
			client.sessionTokenExpiry = timeNow().Add(time.Minute * 15)
		}
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("authentication failed, status code: %v: %s", resp.StatusCode, string(bodyBytes))
	}
	log.Trace("get CSRF token successful")

	return nil
}

func (client *Client) runJSpringSecurityCheck(authPayload *url.Values) error {
	// Request to /j_spring_security_check to obtain CSRF token and session cookie
	req, err := client.newRequest("POST", "/versa/j_spring_security_check", strings.NewReader(authPayload.Encode()), true)
	if err != nil {
		return err
	}

	if client.sessionToken != "" {
		req.Header.Add("X-CSRF-TOKEN", client.sessionToken)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	sessionRes, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	defer sessionRes.Body.Close()

	bodyBytes, err := io.ReadAll(sessionRes.Body)
	if err != nil {
		return err
	}

	endpointURL, err := url.Parse(client.directorEndpoint + "/versa")
	if err != nil {
		return fmt.Errorf("url parsing failed: %w", err)
	}
	cookies := client.httpClient.Jar.Cookies(endpointURL)
	for _, cookie := range cookies {
		if cookie.Name == "VD-CSRF-TOKEN" {
			client.sessionToken = cookie.Value
			client.sessionTokenExpiry = timeNow().Add(time.Minute * 15)
		}
	}

	if sessionRes.StatusCode != 200 {
		return fmt.Errorf("authentication failed, status code: %v: %s", sessionRes.StatusCode, string(bodyBytes))
	}
	log.Trace("j_spring_security_check successful")

	return nil
}

func (client *Client) runAnalyticsLogin(analyticsPayload *url.Values) error {
	// TODO: use proper client request creation, this is a testing work around
	req, err := client.newRequest("POST", "/versa/analytics/login", strings.NewReader(analyticsPayload.Encode()), true)
	if err != nil {
		return err
	}
	req.Header.Add("X-CSRF-TOKEN", client.sessionToken)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	loginRes, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	defer loginRes.Body.Close()

	bodyBytes, err := io.ReadAll(loginRes.Body)
	if err != nil {
		return err
	}

	if loginRes.StatusCode != 200 {
		return fmt.Errorf("analytics authentication failed, status code: %v: %s", loginRes.StatusCode, string(bodyBytes))
	}
	log.Trace("analytics login successful")

	return nil
}
