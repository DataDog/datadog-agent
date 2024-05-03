// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package guiimpl

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

type authenticator struct {
	duration   time.Duration
	signingKey []byte
}

func newAuthenticator(authToken string, duration time.Duration) authenticator {
	return authenticator{
		duration:   duration,
		signingKey: []byte(authToken),
	}
}

// This function check the reveived authToken and return an access token if valid
func (a *authenticator) GenerateAccessToken() string {
	return hmacToken(a.signingKey, time.Now())
}

func (a *authenticator) ValidateToken(token string) error {
	// Split the token into the issued time and HMAC sum
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return fmt.Errorf("invalid token format")
	}

	// Decode the issued time from base64
	unixTimeBytes, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("failed to decode issued time")
	}
	unixTime := int64(binary.LittleEndian.Uint64(unixTimeBytes))
	issuedTime := time.Unix(unixTime, 0)

	// Check if the issued time is older than the duration
	if a.duration != 0 && time.Since(issuedTime) > a.duration {
		return fmt.Errorf("token is expired")
	}

	// Decode the HMAC sum from base64
	hmacSum, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("failed to decode HMAC sum")
	}

	// Calculate the expected HMAC sum
	mac := hmac.New(sha256.New, a.signingKey)
	mac.Write(unixTimeBytes)
	expectedHmacSum := mac.Sum(nil)

	// Check if the HMAC sum matches the expected HMAC sum
	if !hmac.Equal(hmacSum, expectedHmacSum) {
		return fmt.Errorf("invalid token signature")
	}

	return nil
}

func hmacToken(key []byte, issued time.Time) string {
	// Convert the issued time to base64 unixTime format
	unixTimeBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(unixTimeBytes, uint64(issued.Unix()))
	unixTimeBase64 := base64.StdEncoding.EncodeToString(unixTimeBytes)

	// Create the HMAC sum
	mac := hmac.New(sha256.New, key)
	mac.Write(unixTimeBytes)
	hmacSum := mac.Sum(nil)

	// Convert the HMAC sum to base64 format
	hmacBase64 := base64.StdEncoding.EncodeToString(hmacSum)

	// Combine the issued time and HMAC sum with a "." separator
	token := unixTimeBase64 + "." + hmacBase64

	return token
}
