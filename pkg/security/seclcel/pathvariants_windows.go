// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package seclcel

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// No field has path variants on windows. The overrides its file fields carry —
// CaseInsensitiveCmp and WindowsPathCmp — lower case the values and normalise the
// separators rather than reaching for another path, and the overlay and symlink overrides
// are unix only. See pathvariants_unix.go for what a variant is.
var pathVariantReaders = map[string]func(*eval.Context, func(string) bool) bool{}
