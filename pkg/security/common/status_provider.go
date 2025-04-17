// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// NoopStatusProvider is a no-op implementation of the StatusProvider interface
type NoopStatusProvider struct{}

// AddGlobalWarning is a no-op implementation of the StatusProvider interface
func (n *NoopStatusProvider) AddGlobalWarning(string, string) {}

// RemoveGlobalWarning is a no-op implementation of the StatusProvider interface
func (n *NoopStatusProvider) RemoveGlobalWarning(string) {}
