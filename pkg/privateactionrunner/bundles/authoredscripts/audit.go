// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_authoredscripts

import log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"

// authoredScriptAudit holds per-invocation metadata. Fields deliberately exposes
// only package identity and exit code, excluding inputs, environment, output,
// command arguments, and raw errors.
type authoredScriptAudit struct {
	Stage          string
	PackageName    string
	PackageVersion string
	PackageDigest  string
	ExitCode       *int
}

func (a authoredScriptAudit) Fields() []log.Field {
	var fields []log.Field
	if a.PackageName != "" {
		fields = append(fields, log.String("package_name", a.PackageName),
			log.String("package_version", a.PackageVersion),
			log.String("package_digest", a.PackageDigest))
	}
	if a.ExitCode != nil {
		fields = append(fields, log.Int("exit_code", *a.ExitCode))
	}
	return fields
}
