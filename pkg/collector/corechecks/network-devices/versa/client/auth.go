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
	err := client.runJSpringSecurityCheck(&authPayload)
	if err != nil {
		return fmt.Errorf("failed to run j_spring_security_check to get CSRF token: %w", err)
	}
	err = client.runJSpringSecurityCheck(&authPayload)
	if err != nil {
		return fmt.Errorf("failed to run j_spring_security_check to perform login token: %w", err)
	}

	// TODO: can we get a non-HTML response?

	// if len(bodyBytes) > 0 {
	// 	return fmt.Errorf("invalid credentials")
	// }

	// Request to /versa/analytics/login to obtain Analytics CSRF prevention token
	analyticsPayload := url.Values{}
	analyticsPayload.Set("endpoint", "https://10.0.225.103:8443")

	// Run this requrst twice to get the CSRF token from analytics
	// the first succeeds but does not return the token
	err = client.runAnalyticsLogin(&analyticsPayload)
	if err != nil {
		return fmt.Errorf("failed to run FIRST analytics login: %w", err)
	}
	err = client.runAnalyticsLogin(&analyticsPayload)
	if err != nil {
		return fmt.Errorf("failed to run SECOND analytics login: %w", err)
	}

	return nil
}

// TODO: remove this, it's just for manual testing
func (client *Client) Authenticate() error {
	return client.login()
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

func (client *Client) runJSpringSecurityCheck(authPayload *url.Values) error {
	// TODO: this is pretty hacky at the moment, we're investigating
	// how to properly handle the CSRF token and see if we could just
	// use OAuth instead. For now, this gets us off the ground

	// It looks like we may not have to set `_csrf` in the payload, but we'll
	// see

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
	endpointUrl, err := url.Parse(client.endpoint + "/versa")
	if err != nil {
		return fmt.Errorf("url parsing failed: %w", err)
	}

	cookies := client.httpClient.Jar.Cookies(endpointUrl)

	log.Infof("Client login URL: %s", endpointUrl)
	log.Infof("Client login response headers: %+v", sessionRes.Header)
	for _, cookie := range cookies {
		log.Infof("Versa Director cookie: %s=%s;Secure:%T", cookie.Name, cookie.Value, cookie.Secure)
		// TODO: better handling of cookie
		if cookie.Name == "VD-CSRF-TOKEN" {
			client.token = cookie.Value
			client.tokenExpiry = timeNow().Add(time.Hour * 1)
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

	endpointUrl, err := url.Parse(client.endpoint + "/versa")
	if err != nil {
		return fmt.Errorf("url parsing failed: %w", err)
	}

	cookies := client.httpClient.Jar.Cookies(endpointUrl)

	log.Infof("Client ANALYTICS login URL: %s", endpointUrl)
	log.Infof("Client ANALYTICS login response headers: %+v", loginRes.Header)
	for _, cookie := range cookies {
		log.Infof("Versa Analytics cookie: %s=%s;Secure:%t;Path:%s", cookie.Name, cookie.Value, cookie.Secure, cookie.Path)
		// TODO: better handling of cookie
		// if cookie.Name == "VD-CSRF-TOKEN" {
		// 	client.token = cookie.Value
		// 	client.tokenExpiry = timeNow().Add(time.Hour * 1)
		// }
	}

	if loginRes.StatusCode != 200 {
		return fmt.Errorf("analytics authentication failed, status code: %v: %s", loginRes.StatusCode, string(bodyBytes))
	} else {
		log.Infof("Analytics login successful!! %s", string(bodyBytes))
	}

	return nil
}
