// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Login logs in to the Cisco SDWAN API and gets a CSRF prevention token
func (client *Client) login() error {
	authPayload := url.Values{}
	authPayload.Set("j_username", client.username)
	authPayload.Set("j_password", client.password)

	// Request to /j_security_check to obtain session cookie
	req, err := client.newRequest("POST", "/j_security_check", strings.NewReader(authPayload.Encode()))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	sessionRes, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer sessionRes.Body.Close()

	if sessionRes.StatusCode != 200 {
		return fmt.Errorf("authentication failed, status code: %v", sessionRes.StatusCode)
	}

	bodyBytes, err := io.ReadAll(sessionRes.Body)
	if err != nil {
		return err
	}

	if len(bodyBytes) > 0 {
		return fmt.Errorf("invalid credentials")
	}

	// Request to /dataservice/client/token to obtain csrf prevention token
	req, err = client.newRequest("GET", "/dataservice/client/token", nil)
	if err != nil {
		return err
	}
	tokenRes, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer tokenRes.Body.Close()

	if tokenRes.StatusCode != 200 {
		return fmt.Errorf("failed to retrieve csrf prevention token, status code: %v", tokenRes.StatusCode)
	}

	token, _ := io.ReadAll(tokenRes.Body)
	if string(token) == "" {
		return fmt.Errorf("no csrf prevention token in payload")
	}

	client.token = string(token)
	client.tokenExpiry = timeNow().Add(time.Hour * 1)
	return nil
}

// authenticate logins if no token or token is expired
func (client *Client) authenticate() error {
	now := timeNow()

	client.authenticationMutex.Lock()
	defer client.authenticationMutex.Unlock()

	if client.token == "" || client.tokenExpiry.Before(now) {
		return client.login()
	}
	return nil
}

// clearAuth clears auth state
func (client *Client) clearAuth() {
	client.authenticationMutex.Lock()
	client.token = ""
	client.authenticationMutex.Unlock()
}

// isAuthenticated determine if a request was successful from the headers
// Cisco returns HTTP 200 when auth is invalid, with the HTML login page
// API calls returns application/json when successful. We assume receiving HTML means we're unauthenticated.
func isAuthenticated(headers http.Header) bool {
	content := headers.Get("content-type")
	return !strings.HasPrefix(content, "text/html")
}
