// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package milvus

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

// milvusEnv is a thin wrapper around the stock DockerHost so we can attach
// suite methods if we ever want to.
type milvusEnv struct {
	e2e.BaseSuite[environments.DockerHost]
}

// TestProvisioned is a placeholder test method. It exists for one reason:
// testify's Suite runner skips suites that have zero Test* methods
// entirely — it never calls SetupSuite, which is where the e2e-framework
// runs `pulumi up`. Without this method the test "passes" in tens of
// milliseconds and provisions nothing. This method asserts the env was
// initialized; the real assertion that matters is that we got here at
// all (i.e. SetupSuite completed).
func (s *milvusEnv) TestProvisioned() {
	env := s.Env()
	require.NotNil(s.T(), env, "environment should be provisioned")
	require.NotNil(s.T(), env.RemoteHost, "RemoteHost should be provisioned")
	s.T().Logf("provisioned host address = %s", env.RemoteHost.Address)
}

// TestMilvusEnv is intentionally a no-op test: its only purpose is to drive
// the e2e-framework runner to `pulumi up` the Milvus scenario.
//
// Typical usage (deploy only, keep the stack alive for inspection / iteration):
//
//	E2E_DEV_MODE=true \
//	E2E_STACK_NAME=milvus-dev \
//	dda inv new-e2e-tests.run \
//	    --targets=./tests/agent-metric-pipelines/milvus \
//	    --run=^TestMilvusEnv$
//
// Without E2E_DEV_MODE the framework will tear the stack back down once
// the (empty) test returns — useful as a CI-style smoke check that the
// scenario provisions cleanly.
func TestMilvusEnv(t *testing.T) {
	testID := os.Getenv("MILVUS_E2E_TEST_ID")
	if testID == "" {
		testID = randomTestID()
	}
	t.Logf("milvus scenario testID = %s", testID)

	stackName := os.Getenv("E2E_STACK_NAME")
	if stackName == "" {
		stackName = fmt.Sprintf("milvus-%s", testID)
	}

	e2e.Run(t, &milvusEnv{},
		e2e.WithProvisioner(Provisioner(testID)),
		e2e.WithStackName(stackName),
	)
}

func randomTestID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
