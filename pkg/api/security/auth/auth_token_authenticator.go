// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auth

import (
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
)

const authKey = "authorization"

type authTokenGetter func() (string, error)

type authTokenSigner struct {
	getter authTokenGetter
}

// NewAuthTokenSigner provides an implementation of the Authorizer interface.
// It signs and verifies requests using a clear text auth_token.
// The auth_token is retrieved each time using the provided auth_token getter.
func NewAuthTokenSigner(getter authTokenGetter) Authorizer {
	return &authTokenSigner{
		getter: getter,
	}
}

// NewStaticAuthTokenSigner provides an implementation of the Authorizer interface.
// It signs and verifies requests using a clear text auth_token.
func NewStaticAuthTokenSigner(token string) Authorizer {
	return &authTokenSigner{
		getter: func() (string, error) { return token, nil },
	}
}

func (a *authTokenSigner) SignREST(_ string, reqHeaders map[string][]string, _ io.Reader, _ int64) error {
	token, err := a.getter()
	if err != nil {
		return err
	}

	reqHeaders[http.CanonicalHeaderKey(authKey)] = []string{"Bearer " + token}

	return nil
}

func (a *authTokenSigner) VerifyREST(_ string, reqHeaders map[string][]string, _ io.Reader, _ int64) (statusCode, error) {
	token, err := a.getter()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	authHeader, ok := reqHeaders[http.CanonicalHeaderKey(authKey)]
	if !ok {
		return http.StatusUnauthorized, fmt.Errorf("unable to find authorization header")
	}
	if len(authHeader) == 0 {
		return http.StatusUnauthorized, fmt.Errorf("authorization header is empty")
	}
	if !constantCompareStrings(authHeader[0], "Bearer "+token) {
		return http.StatusForbidden, fmt.Errorf("invalid session token")
	}
	return http.StatusOK, nil
}

func (a *authTokenSigner) SignGRPC(_ string, reqMetadata header) error {
	token, err := a.getter()
	if err != nil {
		return err
	}

	reqMetadata[authKey] = []string{"Bearer " + token}

	return nil
}

func (a *authTokenSigner) VerifyGRPC(_ string, reqMetadata header) error {
	token, err := a.getter()
	if err != nil {
		return err
	}

	authHeader, ok := reqMetadata[authKey]
	if !ok {
		return fmt.Errorf("unable to find authorization header")
	}
	if len(authHeader) == 0 {
		return fmt.Errorf("authorization header is empty")
	}
	if !constantCompareStrings(authHeader[0], "Bearer "+token) {
		return fmt.Errorf("invalid session token")
	}
	return nil
}

// constantCompareStrings compares two strings in constant time.
// It uses the subtle.ConstantTimeCompare function from the crypto/subtle package
// to compare the byte slices of the input strings.
// Returns true if the strings are equal, false otherwise.
func constantCompareStrings(src, tgt string) bool {
	return subtle.ConstantTimeCompare([]byte(src), []byte(tgt)) == 1
}
