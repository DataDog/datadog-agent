// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package v1

import (
	"testing"
)

func Test_validateClientIP(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		want    string
		wantErr bool
	}{
		{
			name:    "valid ipv4",
			addr:    "127.0.0.1",
			want:    "127.0.0.1",
			wantErr: false,
		},
		{
			name:    "invalid ipv4",
			addr:    "127.0.0.1.1",
			want:    "",
			wantErr: true,
		},
		{
			name:    "valid ipv6",
			addr:    "2001:db8:1f70::999:de8:7648:6e8",
			want:    "2001:db8:1f70::999:de8:7648:6e8",
			wantErr: false,
		},
		{
			name:    "invalid ipv6",
			addr:    "::1:",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalidate localhost",
			addr:    "localhost",
			want:    "",
			wantErr: true,
		},
		{
			name:    "validate empty",
			addr:    "",
			want:    "",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateClientIP(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateClientIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("validateClientIP() = %v, want %v", got, tt.want)
			}
		})
	}
}
