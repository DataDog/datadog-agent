// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !secrets

// Placeholder for the secrets package when compiled without it

package secrets

// Init placeholder when compiled without the 'secrets' build tag
func Init(command string, arguments []string, timeout int, maxSize int) {}

// Decrypt placeholder when compiled without the 'secrets' build tag
func Decrypt(data []byte) ([]byte, error) {
	return data, nil
}
