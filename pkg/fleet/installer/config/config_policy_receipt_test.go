// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config/policyreceipt"
	"github.com/stretchr/testify/require"
)

func TestPolicyReceiptAfterOrderedOperations(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "delete recreate", true: "partial failure"}[fail], func(t *testing.T) {
			root := t.TempDir()
			r := policyreceipt.Receipt{SchemaVersion: 1, OrgID: 42, PolicyID: "policy", PolicyVersion: 7, SecretRevision: "first01", DeploymentID: "deployment"}
			payload, err := json.Marshal(r)
			require.NoError(t, err)
			ops := Operations{DeploymentID: "deployment", FileOperations: []FileOperation{
				{FileOperationType: FileOperationDelete, FilePath: "/conf.d/redisdb.d/conf.yaml"},
				{FileOperationType: FileOperationMergePatch, FilePath: "/conf.d/redisdb.d/conf.yaml", Patch: []byte(`{"instances":[{"password":"SEC[password]","port":6379}]}`)},
			}}
			if fail {
				ops.FileOperations = append(ops.FileOperations, FileOperation{FileOperationType: FileOperationMergePatch, FilePath: "/not-allowed"})
			}
			require.NoError(t, ReplaceSecrets(&ops, map[string]string{"password": "private-password", policyreceipt.SecretKey: base64.RawStdEncoding.EncodeToString(payload)}))
			encoded, err := json.Marshal(ops)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "first01", "unexported receipt identity must not cross ordinary operations JSON")
			err = ops.Apply(t.Context(), root)
			if fail {
				require.Error(t, err)
				require.Empty(t, policyreceipt.Verify(root, filepath.Join(root, "conf.d/redisdb.d/conf.yaml")))
				return
			}
			require.NoError(t, err)
			filename := filepath.Join(root, "conf.d/redisdb.d/conf.yaml")
			got := policyreceipt.Verify(root, filename)
			require.Len(t, got, 1)
			require.Equal(t, r.SecretRevision, got[0].SecretRevision)
			require.NoError(t, os.WriteFile(filename, []byte("instances:\n- password: drifted-password\n  port: 6379\n"), 0640))
			require.Empty(t, policyreceipt.Verify(root, filename))
		})
	}
}
