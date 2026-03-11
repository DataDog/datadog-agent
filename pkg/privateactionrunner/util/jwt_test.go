// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateKeys(t *testing.T) {
	privateJwk, publicJwk, err := GenerateKeys()
	require.NoError(t, err)

	// Verify private key
	assert.NotNil(t, privateJwk)
	assert.Equal(t, "ES256", privateJwk.Algorithm)
	assert.Equal(t, "sig", privateJwk.Use)
	assert.NotEmpty(t, privateJwk.KeyID)

	// Verify it's a private key
	_, ok := privateJwk.Key.(*ecdsa.PrivateKey)
	assert.True(t, ok)

	// Verify public key
	assert.NotNil(t, publicJwk)
	assert.Equal(t, "ES256", publicJwk.Algorithm)
	assert.Equal(t, "sig", publicJwk.Use)
	assert.NotEmpty(t, publicJwk.KeyID)

	// Verify it's a public key
	_, ok = publicJwk.Key.(*ecdsa.PublicKey)
	assert.True(t, ok)
}

func TestBase64ToJWK(t *testing.T) {
	// Generate a key pair and serialize the private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	privateJwk, err := EcdsaToJWK(privateKey)
	require.NoError(t, err)

	// Serialize to JSON and base64 encode
	jwkBytes, err := json.Marshal(privateJwk)
	require.NoError(t, err)

	encoded := base64.RawURLEncoding.EncodeToString(jwkBytes)

	// Test decoding
	decoded, err := Base64ToJWK(encoded)
	require.NoError(t, err)

	assert.Equal(t, privateJwk.Algorithm, decoded.Algorithm)
	assert.Equal(t, privateJwk.Use, decoded.Use)
	assert.Equal(t, privateJwk.KeyID, decoded.KeyID)
}

func TestBase64ToJWKErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "invalid base64",
			input: "not-valid-base64!!!",
		},
		{
			name:  "valid base64 but invalid JSON",
			input: base64.RawURLEncoding.EncodeToString([]byte("not json")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Base64ToJWK(tt.input)
			assert.Error(t, err)
		})
	}
}

func TestEcdsaToJWK(t *testing.T) {
	t.Run("private key", func(t *testing.T) {
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		jwk, err := EcdsaToJWK(privateKey)
		require.NoError(t, err)

		assert.Equal(t, "ES256", jwk.Algorithm)
		assert.Equal(t, "sig", jwk.Use)
		assert.NotEmpty(t, jwk.KeyID)
		assert.Equal(t, privateKey, jwk.Key)
	})

	t.Run("public key", func(t *testing.T) {
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		jwk, err := EcdsaToJWK(&privateKey.PublicKey)
		require.NoError(t, err)

		assert.Equal(t, "ES256", jwk.Algorithm)
		assert.Equal(t, "sig", jwk.Use)
		assert.NotEmpty(t, jwk.KeyID)
		assert.Equal(t, &privateKey.PublicKey, jwk.Key)
	})

	t.Run("unsupported key type", func(t *testing.T) {
		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		_, err = EcdsaToJWK(rsaKey)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported key type")
	})

	t.Run("string type", func(t *testing.T) {
		_, err := EcdsaToJWK("not a key")
		assert.Error(t, err)
	})
}

func TestGeneratePARJWT(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	t.Run("basic JWT generation", func(t *testing.T) {
		orgID := int64(12345)
		runnerID := "test-runner"

		tokenString, err := GeneratePARJWT(orgID, runnerID, privateKey, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, tokenString)

		// Verify the token has three parts (header.payload.signature)
		parts := strings.Split(tokenString, ".")
		assert.Len(t, parts, 3)

		// Parse and verify the token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return &privateKey.PublicKey, nil
		})
		require.NoError(t, err)
		assert.True(t, token.Valid)

		// Verify claims
		claims, ok := token.Claims.(jwt.MapClaims)
		require.True(t, ok)

		assert.Equal(t, float64(orgID), claims["orgId"])
		assert.Equal(t, runnerID, claims["runnerId"])
		assert.NotNil(t, claims["iat"])
		assert.NotNil(t, claims["exp"])
	})

	t.Run("with extra claims", func(t *testing.T) {
		orgID := int64(67890)
		runnerID := "runner-2"
		extraClaims := map[string]any{
			"custom": "value",
			"count":  42,
		}

		tokenString, err := GeneratePARJWT(orgID, runnerID, privateKey, extraClaims)
		require.NoError(t, err)

		// Parse and verify extra claims
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return &privateKey.PublicKey, nil
		})
		require.NoError(t, err)

		claims, ok := token.Claims.(jwt.MapClaims)
		require.True(t, ok)

		assert.Equal(t, "value", claims["custom"])
		assert.Equal(t, float64(42), claims["count"])
	})

	t.Run("verify header", func(t *testing.T) {
		tokenString, err := GeneratePARJWT(12345, "runner", privateKey, nil)
		require.NoError(t, err)

		token, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
		require.NoError(t, err)

		assert.Equal(t, "ES256", token.Header["alg"])
		assert.Equal(t, "JWT", token.Header["cty"])
	})
}

func TestJWKToPEM(t *testing.T) {
	t.Run("valid public key", func(t *testing.T) {
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		publicJwk, err := EcdsaToJWK(&privateKey.PublicKey)
		require.NoError(t, err)

		pemString, err := JWKToPEM(publicJwk)
		require.NoError(t, err)

		assert.Contains(t, pemString, "-----BEGIN PUBLIC KEY-----")
		assert.Contains(t, pemString, "-----END PUBLIC KEY-----")
	})

	t.Run("private key fails", func(t *testing.T) {
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		privateJwk, err := EcdsaToJWK(privateKey)
		require.NoError(t, err)

		_, err = JWKToPEM(privateJwk)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not public")
	})

	t.Run("wrong key type fails", func(t *testing.T) {
		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		// Manually create a JWK with RSA key
		jwk := &jose.JSONWebKey{
			Key: &rsaKey.PublicKey,
		}

		_, err = JWKToPEM(jwk)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "wrong underlying key type")
	})
}
