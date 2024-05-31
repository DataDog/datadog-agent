// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"testing"

	"go.uber.org/zap"
)

func TestServiceNameFromCLI(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "should return laravel for artisan commands",
			args:     []string{"php", "artisan", "serve"},
			expected: "laravel",
		},
		{
			name:     "should return service_name for php -ddatadog.service=service_name",
			args:     []string{"php", "-ddatadog.service=service_name", "server.php"},
			expected: "service_name",
		},
		{
			name:     "should return service_name for php -d datadog.service=service_name",
			args:     []string{"php", "-d", "datadog.service=service_name", "server.php"},
			expected: "service_name",
		},
		{
			name:     "Nothing found",
			args:     []string{"php", "server.php"},
			expected: "",
		},
	}
	instance := &phpDetector{ctx: NewDetectionContext(zap.NewExample(), nil, nil, nil)}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := instance.detect(tt.args)
			if len(tt.expected) > 0 {
				if !ok {
					t.Errorf("expected ok to be true, got false")
				}
				if value.Name != tt.expected {
					t.Errorf("expected %s, got %s", tt.expected, value.Name)
				}
			} else {
				if ok {
					t.Errorf("expected ok to be false, got true")
				}
			}
		})
	}
}
