// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package guiimpl

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type authenticator struct {
	sessionId  string
	signingKey []byte
}

func newAuthenticator(authToken string) authenticator {
	return authenticator{
		sessionId:  uuid.New().String(),
		signingKey: []byte(authToken),
	}
}

// This function check the reveived authToken and return an access token if valid
func (a *authenticator) Authenticate(rawToken string) (string, error) {
	// multiple checks on provided token
	// - token is signed with Agent authToken using HMAC SHA256 alg
	// - token is not expired (set to 1 minute in agent launch-gui command)
	_, err := jwt.Parse(
		rawToken,
		a.getSigningKey,
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{"HS256"}),
	)
	if err != nil {
		return "", err
	}

	// Create the accessToken that the user will register as a cookie
	// with the current session ID
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwt.RegisteredClaims{
		Subject: a.sessionId,
	})

	// Sign and get the complete encoded token as a string using the secret
	accessTokenString, err := accessToken.SignedString(a.signingKey)
	if err != nil {
		return "", fmt.Errorf("Internal error")
	}
	return accessTokenString, nil
}

func (a *authenticator) Authorize(rawToken string) error {
	// multiple checks on provided token
	// - token is signed with Agent authToken using HMAC SHA256 alg
	// - token is from the same session ID as the current one
	_, err := jwt.Parse(
		rawToken,
		a.getSigningKey,
		jwt.WithSubject(a.sessionId),
		jwt.WithValidMethods([]string{"HS256"}),
	)

	if err != nil {
		return err
	}

	return nil
}

func (a *authenticator) getSigningKey(_ *jwt.Token) (interface{}, error) {
	return a.signingKey, nil
}
