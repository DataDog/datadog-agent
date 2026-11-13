// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"regexp"
	"time"

	"k8s.io/apimachinery/pkg/util/cache"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	warningCacheSize   = 1000
	warningLogInterval = time.Hour
)

var (
	supressedWarning     = regexp.MustCompile(`.*is deprecated in v.*`)
	defaultWarningLogger = NewCustomWarningLogger()
)

// CustomWarningLogger logs the warnings returned by the API server, each distinct
// message at most once per warningLogInterval.
type CustomWarningLogger struct {
	logged *cache.LRUExpireCache
}

// NewCustomWarningLogger returns a CustomWarningLogger.
func NewCustomWarningLogger() *CustomWarningLogger {
	return &CustomWarningLogger{logged: cache.NewLRUExpireCache(warningCacheSize)}
}

// HandleWarningHeader logs a warning returned by the API server, unless it is suppressed.
func (l *CustomWarningLogger) HandleWarningHeader(code int, _ string, message string) {
	if code != 299 || len(message) == 0 || supressedWarning.MatchString(message) {
		return
	}

	if _, found := l.logged.Get(message); found {
		log.Debugf("Kubernetes API server warning: %s", message)
		return
	}
	l.logged.Add(message, struct{}{}, warningLogInterval)
	log.Infof("Kubernetes API server warning: %s", message)
}
