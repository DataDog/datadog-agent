// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/sds"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/testcommon/check"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

// postgresAccountsSeedRows is the number of rows inserted into `accounts` by
// postgres-init.sql. The workload generator updates balances but does not add
// or delete account rows.
const postgresAccountsSeedRows int64 = 200

// postgresScanEnv is a host Agent plus a Dockerized PostgreSQL workload on the same VM.
type postgresScanEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	FakeIntake *components.FakeIntake
	Docker     *components.RemoteHostDocker
}

type postgresScanSuite struct {
	e2e.BaseSuite[postgresScanEnv]
}

func TestDataSecurityPostgresScan(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &postgresScanSuite{}, e2e.WithPulumiProvisioner(postgresScanProvisioner(), nil))
}

func (s *postgresScanSuite) TestPackagedCheckLoadsAndEmitsSDSResult() {
	// The packaged check loads and runs without error. Parse the check output
	// with the shared testcommon helper — no manual struct parsing.
	out, err := s.Env().Agent.Client.CheckWithError(agentclient.WithArgs([]string{"datasecurity", "--json", "--delay", "1000"}))
	require.NoError(s.T(), err, "datasecurity check failed: %s", out)
	assert.NotContains(s.T(), out, "'group' or 'others' have rights on it")
	assert.NotContains(s.T(), out, "no valid check found")

	data := check.ParseJSONOutput(s.T(), []byte(out))
	require.NotEmpty(s.T(), data, "empty check JSON: %s", out)
	assert.Equal(s.T(), 0, data[0].Runner.TotalErrors, "datasecurity check reported errors")

	// The scheduled check forwards one sds-result to fakeintake.
	fakeintake := s.Env().FakeIntake.Client()
	expected := expectedPostgresSDSResult()
	s.EventuallyWithT(func(c *assert.CollectT) {
		payloads, err := fakeintake.GetSDSResults()
		require.NoError(c, err)
		// The check has a single sub task, so it emits exactly one sds-result.
		require.Len(c, payloads, 1, "expected exactly one sds-result payload")

		got := proto.Clone(&payloads[0].SdsResultPayload).(*sds.SdsResultPayload)
		require.Greater(c, got.GetTimestamp(), int64(0), "timestamp should be populated")
		got.Timestamp = 0
		if !proto.Equal(expected, got) {
			assert.Fail(c, "sds-result payload did not match",
				"want:\n%s\ngot:\n%s", protojson.Format(expected), protojson.Format(got))
		}
	}, 2*time.Minute, 10*time.Second)
}

func expectedPostgresSDSResult() *sds.SdsResultPayload {
	return &sds.SdsResultPayload{
		Resource: &sds.SdsResultPayload_Resource{
			Type: "postgres_table",
			Name: "e2e-instance.labdb.public.accounts",
		},
		RuleIds: []string{"e2e-owner-pattern"},
		ScanningSource: &sds.ScanningSource{
			Source: &sds.ScanningSource_Agent_{
				Agent: &sds.ScanningSource_Agent{},
			},
		},
		ScanResults: []*sds.SdsResultPayload_ScanResult{{
			TableMatches: []*sds.SdsResultPayload_TableMatch{{
				RuleId:           "e2e-owner-pattern",
				ColumnName:       "owner",
				CountMatchedRows: postgresAccountsSeedRows,
				CountMatches:     postgresAccountsSeedRows,
			}},
			Location: &sds.SdsResultPayload_ScanLocation{
				ScanLocation: &sds.SdsResultPayload_ScanLocation_PostgresTable{
					PostgresTable: &sds.SdsResultPayload_PostgresTable{
						DatabaseClusterName:  "e2e-cluster",
						DatabaseInstanceName: "e2e-instance",
						DatabaseHostName:     "localhost",
						DatabaseName:         "labdb",
						SchemaName:           "public",
						TableName:            "accounts",
						ScannedRowCount:      postgresAccountsSeedRows,
						ScannedColumns: []*sds.SdsResultPayload_PostgresTable_ScannedColumn{{
							Name:     "owner",
							DataType: "text",
						}},
					},
				},
			},
			ScanMetadata: &sds.SdsResultPayload_ScanMetadata{
				ScanTaskMetadata: &sds.SdsResultPayload_ScanMetadata_ScanTaskMetadata{
					TaskId:    "e2e-datasec-postgres",
					SubTaskId: "e2e-accounts",
					Status:    sds.SdsResultPayload_ScanMetadata_ScanTaskMetadata_SUCCESS,
				},
			},
		}},
	}
}
