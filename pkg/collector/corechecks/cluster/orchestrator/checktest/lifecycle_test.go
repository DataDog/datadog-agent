// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package checktest

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

func TestLifecycle(t *testing.T) {
	// The check sends the deletions that it queued when the Agent stops it, as no later run would.
	t.Run("queued_deletions_are_sent_when_the_check_stops", func(t *testing.T) {
		env := suite.Setup(t)
		uids := env.UIDs()

		env.AssertPop(t, firstRun(uids)...)
		env.DeleteAll(t)
		env.WaitForDeletions(t, deletion(uids)...)
		suite.AssertObjects(t, deletion(uids), env.Stop(t))
	})

	// A check restarted in the same process, like a check whose configuration changes, shares the cache of the resource
	// versions sent: it sends the unchanged objects again only when their cache entries expire.
	t.Run("restart_in_the_same_process", func(t *testing.T) {
		env := suite.Setup(t)
		uids := env.UIDs()

		env.AssertPop(t, firstRun(uids)...)
		suite.AssertObjects(t, nil, env.Restart(t))
		env.AssertPop(t)
		env.ExpireResourceCache()
		env.AssertPop(t, firstRun(uids)...)
	})
}
