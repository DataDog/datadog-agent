// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package guiimpl

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/dev/test/github.com/stretchr/testify/require"
)

func TestAuthenticator(t *testing.T) {
	var key = "secretKey"
	auth := newAuthenticator(key, 5*time.Minute)

	tests := []struct {
		name  string
		token string
		err   error
	}{
		{
			name:  "random string",
			token: "randomstring",
			err:   fmt.Errorf("invalid token format"),
		},
		{
			name:  "malformed issued time",
			token: "abcded.12345",
			err:   fmt.Errorf("failed to decode issued time"),
		},
		{
			name:  "expired token",
			token: hmacToken([]byte(key), time.Now().Add(-6*time.Minute)),
			err:   fmt.Errorf("token is expired"),
		},
		{
			name:  "not base64 formatted sum",
			token: hmacToken([]byte(key), time.Now()) + "?",
			err:   fmt.Errorf("failed to decode HMAC sum"),
		},
		{
			name: "modified issued time",
			token: func() string {
				oldToken := hmacToken([]byte(key), time.Now().Add(-6*time.Minute))
				parts := strings.Split(oldToken, ".")
				unixTimeBytes := make([]byte, 8)
				binary.LittleEndian.PutUint64(unixTimeBytes, uint64(time.Now().Unix()))
				unixTimeBase64 := base64.StdEncoding.EncodeToString(unixTimeBytes)
				parts[0] = unixTimeBase64

				return parts[0] + "." + parts[1]
			}(),
			err: fmt.Errorf("invalid token signature"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := auth.ValidateToken(tt.token)
			require.Equal(t, tt.err, err)
		})
	}
}
