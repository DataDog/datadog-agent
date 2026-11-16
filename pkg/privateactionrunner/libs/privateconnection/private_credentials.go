// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package privateconnection

import (
	"errors"
)

type AuthType string

const (
	TokenAuthType AuthType = "Token Auth"
	BasicAuthType AuthType = "Basic Auth"
)

type PrivateCredentials struct {
	Type        AuthType
	Tokens      []PrivateCredentialsToken
	HttpDetails HttpDetails
}

type PrivateCredentialsToken struct {
	Name  string
	Value string
}

type HttpDetails struct {
	BaseURL       string
	Headers       []PrivateCredentialsToken
	UrlParameters []PrivateCredentialsToken
	Body          HttpDetailsBody
	Testing       *HttpDetailsTesting
	// OAuth2 is set when the connection gets its bearer token with OAuth2.
	OAuth2 *OAuth2Details
}

const (
	OAuth2ClientCredentialsGrant = "client_credentials"

	OAuth2ClientSecretBasic = "client_secret_basic"
	OAuth2ClientSecretPost  = "client_secret_post"
)

// OAuth2Details configures how the runner gets a bearer token for the
// connection's base URL host. ClientSecret is resolved on the runner.
type OAuth2Details struct {
	GrantType        string
	TokenURL         string
	ClientID         string
	ClientSecret     string
	Scopes           []string
	ClientAuthMethod string
}

type HttpDetailsTesting struct {
	Path string
	Verb string
}

type HttpDetailsBody struct {
	Content     string
	ContentType string
}

func (p PrivateCredentialsToken) GetNameSegments() []string {
	return []string{p.Name}
}

func (p PrivateCredentials) GetUsernamePasswordBasicAuth() (string, string, error) {
	if p.Type != BasicAuthType {
		return "", "", errors.New("not a basic auth credential")
	}
	tokens := p.AsTokenMap()
	return tokens[UsernameTokenName], tokens[PasswordTokenName], nil
}

func (p PrivateCredentials) AsTokenMap() map[string]string {
	tokenMap := make(map[string]string)
	for _, token := range p.Tokens {
		tokenMap[token.Name] = token.Value
	}
	return tokenMap
}
