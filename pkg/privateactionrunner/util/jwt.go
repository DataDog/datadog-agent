// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
)

func GenerateKeys() (*jose.JSONWebKey, *jose.JSONWebKey, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate key pair: %w", err)
	}
	publicKey := &privateKey.PublicKey

	privateJwk, err := EcdsaToJWK(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode private key: %w", err)
	}

	publicJwk, err := EcdsaToJWK(publicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode public key: %w", err)
	}

	return privateJwk, publicJwk, nil
}

func Base64ToJWK(privateKey string) (jwk jose.JSONWebKey, err error) {
	decodedKeyBytes, err := base64.RawURLEncoding.DecodeString(privateKey)
	if err != nil {
		return jose.JSONWebKey{}, fmt.Errorf("error decoding private key: %+v", err)
	}
	if err = json.Unmarshal(decodedKeyBytes, &jwk); err != nil {
		return jose.JSONWebKey{}, fmt.Errorf("error converting private key to JWK: %+v", err)
	}
	return jwk, nil
}

func EcdsaToJWK(key any) (*jose.JSONWebKey, error) {
	// Check if the key is a ECDSA key.
	switch key.(type) {
	case *ecdsa.PrivateKey, *ecdsa.PublicKey:
	default:
		return nil, errors.New("unsupported key type")
	}

	// Encode the public key.
	newJwk := jose.JSONWebKey{
		Algorithm: "ES256",
		Key:       key,
		Use:       "sig",
	}

	// Compute the thumbprint of the public key.
	thumbprint, err := newJwk.Thumbprint(crypto.SHA256)
	if err != nil {
		return nil, err
	}

	// Set the key id.
	newJwk.KeyID = base64.RawURLEncoding.EncodeToString(thumbprint)

	return &newJwk, nil
}

func GeneratePARJWT(orgId int64, runnerId string, privateKey *ecdsa.PrivateKey, extraClaims map[string]any) (string, error) {
	claims := jwt.MapClaims{
		"orgId":    orgId,
		"runnerId": runnerId,
		"iat":      time.Now().Unix(), // this was added after version 1.7.0
		"exp":      time.Now().Add(time.Minute * 1).Unix(),
	}

	maps.Copy(claims, extraClaims)

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["alg"] = "ES256"
	token.Header["cty"] = "JWT"

	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("error signing JWT: %w", err)
	}

	return signed, nil
}

// JWKToPEM converts a JWK public key to PEM format
func JWKToPEM(pubJWK *jose.JSONWebKey) (string, error) {
	if !pubJWK.IsPublic() {
		return "", errors.New("error converting JWK to PEM: the key is not public")
	}

	pk, ok := pubJWK.Key.(*ecdsa.PublicKey)
	if !ok {
		return "", errors.New("error converting JWK to PEM: wrong underlying key type")
	}

	x509EncodedPub, err := x509.MarshalPKIXPublicKey(pk)
	if err != nil {
		return "", errors.New("error converting JWK to PEM: failed to marshal public key")
	}

	pemEncodedPub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: x509EncodedPub})
	if pemEncodedPub == nil {
		return "", errors.New("error converting JWK to PEM: failed to encode public key to PEM format")
	}

	return string(pemEncodedPub), nil
}
