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

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Login logs in to the Versa Director API, Versa Analytics API, gets CSRF
// tokens, and a session cookie
func (client *Client) login() error {
	authPayload := url.Values{}
	authPayload.Set("j_username", client.username)
	authPayload.Set("j_password", client.password)
	// this is a hack to get a CSRF token then
	// actually perform login
	//
	// ideally, we'd have a different endpoint to get a CSRF token
	// from at the very least
	err := client.runJSpringSecurityCheck(&authPayload)
	if err != nil {
		return fmt.Errorf("failed to run j_spring_security_check to get CSRF token: %w", err)
	}

	// now we can actually login and get a session cookie
	err = client.runJSpringSecurityCheck(&authPayload)
	if err != nil {
		return fmt.Errorf("failed to run j_spring_security_check to get session token: %w", err)
	}

	// Request to /versa/analytics/login to obtain Analytics CSRF prevention token
	analyticsPayload := url.Values{}
	analyticsPayload.Set("endpoint", client.analyticsEndpoint) // TODO: WHY? Where can we get this for others?

	// Run this request twice to get the CSRF token from analytics
	// the first succeeds but does not return the token
	err = client.runAnalyticsLogin(&analyticsPayload)
	if err != nil {
		return fmt.Errorf("failed to run analytics login: %w", err)
	}
	err = client.runAnalyticsLogin(&analyticsPayload)
	if err != nil {
		return fmt.Errorf("failed to get analytics CSRF token: %w", err)
	}

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
// Vera can return HTTP 200 when auth is invalid, with the HTML login page
// API calls returns application/json when successful. We assume receiving HTML means we're unauthenticated.
func isAuthenticated(headers http.Header) bool {
	content := headers.Get("content-type")
	return !strings.HasPrefix(content, "text/html")
}

func (client *Client) runJSpringSecurityCheck(authPayload *url.Values) error {
	// TODO: this is pretty hacky at the moment, we're investigating
	// how to properly handle the CSRF token and see if we could just
	// use OAuth instead. For now, this gets us off the ground

	// Request to /j_spring_security_check to obtain CSRF token and session cookie
	req, err := client.newRequest("POST", "/versa/j_spring_security_check", strings.NewReader(authPayload.Encode()))
	if err != nil {
		return err
	}

	// if we have a CSRF token, add it to the request
	if client.token != "" {
		req.Header.Add("X-CSRF-TOKEN", client.token)
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

	// TODO: remove this, we don't need it, just using it for debugging
	endpointURL, err := url.Parse(client.endpoint + "/versa")
	if err != nil {
		return fmt.Errorf("url parsing failed: %w", err)
	}

	cookies := client.httpClient.Jar.Cookies(endpointURL)

	log.Tracef("Client login URL: %s", endpointURL)
	log.Tracef("Client login response headers: %+v", sessionRes.Header)
	for _, cookie := range cookies {
		log.Tracef("Versa Director cookie: %s=%s;Secure:%T", cookie.Name, cookie.Value, cookie.Secure)
		// TODO: replace with OAuth token
		if cookie.Name == "VD-CSRF-TOKEN" {
			client.token = cookie.Value
			client.tokenExpiry = timeNow().Add(time.Minute * 15)
		}
	}

	if sessionRes.StatusCode != 200 {
		return fmt.Errorf("authentication failed, status code: %v: %s", sessionRes.StatusCode, string(bodyBytes))
	}

	return nil
}

func (client *Client) runAnalyticsLogin(analyticsPayload *url.Values) error {
	req, err := client.newRequest("POST", "/versa/analytics/login", strings.NewReader(analyticsPayload.Encode()))
	if err != nil {
		return err
	}
	req.Header.Add("X-CSRF-TOKEN", client.token)
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

	endpointURL, err := url.Parse(client.endpoint + "/versa")
	if err != nil {
		return fmt.Errorf("url parsing failed: %w", err)
	}

	cookies := client.httpClient.Jar.Cookies(endpointURL)

	log.Tracef("Client ANALYTICS login URL: %s", endpointURL)
	log.Tracef("Client ANALYTICS login response headers: %+v", loginRes.Header)
	for _, cookie := range cookies {
		log.Tracef("Versa Analytics cookie: %s=%s;Secure:%t;Path:%s", cookie.Name, cookie.Value, cookie.Secure, cookie.Path)
	}

	if loginRes.StatusCode != 200 {
		return fmt.Errorf("analytics authentication failed, status code: %v: %s", loginRes.StatusCode, string(bodyBytes))
	}
	log.Tracef("Analytics login successful!! %s", string(bodyBytes))

	return nil
}
