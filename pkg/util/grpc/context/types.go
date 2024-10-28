// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package context provides some constant contexts
package context

type contextKey struct {
	key string
}

// ConnContextKey is a contextKey with http-connection key
var ConnContextKey = &contextKey{"http-connection"}

// ContextKeyTokenInfoID is a contextKey with token-info-id key
var ContextKeyTokenInfoID = &contextKey{"token-info-id"}
