// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"github.com/elastic/go-libaudit/rule"
)

// AuditClient defines the interface for interacting with the auditd client
type AuditClient interface {
	GetFileWatchRules() ([]*rule.FileWatchRule, error)
	Close() error
}
