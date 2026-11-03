// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_authoredscripts

import (
	"testing"

	"github.com/stretchr/testify/require"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
)

func TestAuthoredScriptAuditFields(t *testing.T) {
	exitCode := 0
	for _, tc := range []struct {
		name  string
		audit authoredScriptAudit
		want  []log.Field
	}{
		{name: "unresolved package", audit: authoredScriptAudit{Stage: "catalog"}},
		{
			name:  "resolved package",
			audit: authoredScriptAudit{Stage: "execution", PackageName: "script", PackageVersion: "1.2.3", PackageDigest: "sha256:abc"},
			want:  []log.Field{log.String("package_name", "script"), log.String("package_version", "1.2.3"), log.String("package_digest", "sha256:abc")},
		},
		{name: "available exit code", audit: authoredScriptAudit{ExitCode: &exitCode}, want: []log.Field{log.Int("exit_code", 0)}},
		{name: "unknown package", audit: authoredScriptAudit{PackageVersion: "1.2.3", PackageDigest: "sha256:abc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.audit.Fields())
		})
	}
}
