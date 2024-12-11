// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package guiimpl

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAuthenticator(t *testing.T) {
	var key = "secretKey"
	auth := newAuthenticator(key, 5*time.Minute)

	tests := []struct {
		name   string
		token  string
		errMsg string
	}{
		{
			name:   "valid token",
			token:  hmacToken([]byte(key), time.Now(), time.Now().Add(5*time.Minute)),
			errMsg: "",
		},
		{
			name:   "wrong key token",
			token:  hmacToken([]byte("wrongkey"), time.Now(), time.Now().Add(5*time.Minute)),
			errMsg: "invalid token signature",
		},
		{
			name:   "random string",
			token:  "randomstring",
			errMsg: "invalid token format",
		},
		{
			name:   "malformed issued time",
			token:  "v1.abcded.12345",
			errMsg: "failed to decode payload",
		},
		{
			name:   "expired token",
			token:  hmacToken([]byte(key), time.Now().Add(-6*time.Minute), time.Now().Add(-1*time.Minute)),
			errMsg: "token is expired",
		},
		{
			name:   "not base64 formatted sum",
			token:  hmacToken([]byte(key), time.Now(), time.Now().Add(5*time.Minute)) + "?",
			errMsg: "failed to decode HMAC sum",
		},
		{
			name: "modified issued time",
			token: func() string {
				oldToken := hmacToken([]byte(key), time.Now().Add(-6*time.Minute), time.Now().Add(5*time.Minute))
				parts := strings.Split(oldToken, ".")
				payloadBytes := make([]byte, 16)
				binary.LittleEndian.PutUint64(payloadBytes, uint64(time.Now().Unix()))
				binary.LittleEndian.PutUint64(payloadBytes[8:], uint64(time.Now().Add(5*time.Minute).Unix()))
				payloadBase64 := base64.StdEncoding.EncodeToString(payloadBytes)
				parts[1] = payloadBase64

				return parts[0] + "." + parts[1] + "." + parts[2]
			}(),
			errMsg: "invalid token signature",
		},
		{
			name:   "wrong version",
			token:  "_" + hmacToken([]byte(key), time.Now(), time.Now().Add(5*time.Minute)),
			errMsg: "token version mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := auth.ValidateToken(tt.token)

			if tt.errMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.errMsg)
			}
		})
	}
}
