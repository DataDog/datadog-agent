// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package client

import (
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestClaims(t *testing.T) {
	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "1",
			Issuer:    "cluster-agent",
			Subject:   "container-inspection",
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
	}

	claimToken := jwt.NewWithClaims(signingMethod, claims)
	ss, err := claimToken.SignedString([]byte("test-signing-key"))
	require.NoError(t, err)

	myClaim, err := ParseClaimsString(ss, keyFuncForSecret([]byte("test-signing-key")))
	require.NoError(t, err)

	issuedAt, err := myClaim.GetIssuedAt()
	require.NoError(t, err)
	require.NotNil(t, issuedAt)
	require.Equal(t, claims.IssuedAt, issuedAt)
	require.Equal(t, claims.ExpiresAt, myClaim.ExpiresAt)
	require.True(t, myClaim.isTimeValid(now.Add(1*time.Second)))
	require.False(t, myClaim.isTimeValid(now.Add(-1*time.Second)))
	require.False(t, myClaim.isTimeValid(now.Add(5*time.Minute)))

	_, err = ParseClaimsString(ss, keyFuncForSecret([]byte("bad-signing-key")))
	require.Error(t, err)

	badAlgoSigned, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).
		SignedString([]byte("test-signing-key"))
	require.NoError(t, err)

	_, err = ParseClaimsString(badAlgoSigned, keyFuncForSecret([]byte("test-signing-key")))
	require.Error(t, err)
}

func TestInvalidClaimsTimes(t *testing.T) {
	now := time.Now()
	validationTime := now.Add(37 * time.Second)

	tests := []struct {
		name   string
		claims *Claims
	}{
		{
			name:   "empty",
			claims: &Claims{},
		},
		{
			name: "no expiresAt",
			claims: &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					IssuedAt:  jwt.NewNumericDate(now),
					NotBefore: jwt.NewNumericDate(now),
				},
			},
		},
		{
			name: "notBefore is in the future",
			claims: &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					IssuedAt:  jwt.NewNumericDate(now),
					NotBefore: jwt.NewNumericDate(now.Add(1 * time.Hour)),
					ExpiresAt: jwt.NewNumericDate(now.Add(2 * time.Hour)),
				},
			},
		},
		{
			name: "already expired",
			claims: &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					IssuedAt:  jwt.NewNumericDate(now),
					NotBefore: jwt.NewNumericDate(now),
					ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Second)),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.False(t, tt.claims.isTimeValid(validationTime))
		})
	}

}
