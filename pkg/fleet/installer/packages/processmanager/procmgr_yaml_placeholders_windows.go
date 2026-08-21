// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package processmanager

import (
	"path/filepath"
	"strings"
)

// yamlSingleQuoteContent escapes s for embedding inside a YAML single-quoted scalar.
// Windows procmgr templates quote placeholders at codegen; real install/config roots
// are substituted at install time and must be escaped here.
func yamlSingleQuoteContent(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func substituteProcmgrYAMLPlaceholders(config, placeholderPrefix, installRoot, etcRoot string) string {
	installRootRepl := yamlSingleQuoteContent(filepath.ToSlash(filepath.Clean(installRoot)))
	etcRootRepl := yamlSingleQuoteContent(filepath.ToSlash(filepath.Clean(etcRoot)))
	config = strings.ReplaceAll(config, "__"+placeholderPrefix+"_INSTALL_ROOT__", installRootRepl)
	config = strings.ReplaceAll(config, "__"+placeholderPrefix+"_ETC_ROOT__", etcRootRepl)
	return config
}
