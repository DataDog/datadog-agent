// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package domainresolver

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_domainResolver_getIPToDomainMap(t *testing.T) {
	lookupHostFn := func(host string) ([]string, error) {
		switch host {
		case "dns.google.com":
			return []string{"8.8.8.8", "8.8.4.4", "2001:4860:4860::8844", "2001:4860:4860::8888"}, nil
		case "zoom.us":
			return []string{"170.114.52.2", "2407:30c0:182::aa72:3402"}, nil
		case "error":
			return nil, errors.New("test error")
		}
		return nil, nil
	}
	tests := []struct {
		name                  string
		domains               []string
		expectedIPToDomainMap map[string]string
		expectedErrList       []error
	}{
		{
			name:    "valid domains",
			domains: []string{"dns.google.com", "zoom.us"},
			expectedIPToDomainMap: map[string]string{
				"170.114.52.2":             "zoom.us",
				"2407:30c0:182::aa72:3402": "zoom.us",
				"8.8.4.4":                  "dns.google.com",
				"8.8.8.8":                  "dns.google.com",
				"2001:4860:4860::8844":     "dns.google.com",
				"2001:4860:4860::8888":     "dns.google.com",
			},
		},
		{
			name:    "error case",
			domains: []string{"error", "zoom.us"},
			expectedIPToDomainMap: map[string]string{
				"170.114.52.2":             "zoom.us",
				"2407:30c0:182::aa72:3402": "zoom.us",
			},
			expectedErrList: []error{errors.New("error looking up IPs for domain error: test error")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DomainResolver{
				lookupHostFn: lookupHostFn,
			}
			ipToDomainMap, errList := d.getIPToDomainMap(tt.domains)
			assert.Equal(t, tt.expectedIPToDomainMap, ipToDomainMap)
			assert.Equal(t, errors.Join(tt.expectedErrList...), errors.Join(errList...))
		})
	}
}
