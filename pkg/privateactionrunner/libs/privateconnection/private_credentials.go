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
