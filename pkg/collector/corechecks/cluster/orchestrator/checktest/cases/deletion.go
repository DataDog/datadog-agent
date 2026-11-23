// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package cases

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

func init() { suite.Register("Deletion", deletion) }

// deletion runs the tests of deleted resources that the informers report late or that the check queued.
func deletion(t *testing.T, cs *suite.Suite) {
	cs.RunCases(t,
		// A deletion that an informer missed, which it reports after a relist with the last known state of the
		// object, is sent as a terminated resource. The pods collector ignores deletions: the pod deletion watcher
		// tests cover the deletions that it missed.
		suite.Case{Name: "MissedDeletion", Steps: []suite.Step{
			suite.FirstRun,
			{
				Name: "a relist reports the missed deletion of every object",
				Act: func(_ *testing.T, sc *suite.Scenario) {
					for _, o := range sc.Collected() {
						if !sc.Expected.Live[o.UID()].Deletes {
							continue
						}
						for _, collector := range sc.Collectors(o.FullName) {
							collector.QueueTerminated(cache.DeletedFinalStateUnknown{Key: key(o.Object), Obj: o.Object})
						}
					}
				},
				Want: func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads {
					var deletes []suite.ScenarioObject
					for _, o := range sc.Collected() {
						if sc.Expected.Live[o.UID()].Deletes {
							deletes = append(deletes, o)
						}
					}
					return sc.TerminatedOf(t, deletes)
				},
			},
		}},
		// Cancelling the check sends the deletions that it queued since its last run.
		suite.Case{Name: "CancelSendsQueuedDeletions", Steps: []suite.Step{
			suite.FirstRun,
			{
				Name: "delete every object and cancel the check",
				Act: func(t *testing.T, sc *suite.Scenario) {
					sc.DeleteAll(t, sc.Collected())
					sc.WaitQueued(t, sc.Collected())
				},
				Run: suite.CancelCheck,
				Want: func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads {
					return sc.TerminatedOf(t, sc.Collected())
				},
			},
		}},
	)
}

// key returns the informer key of an object.
func key(object runtime.Object) string {
	key, _ := cache.MetaNamespaceKeyFunc(object)
	return key
}
