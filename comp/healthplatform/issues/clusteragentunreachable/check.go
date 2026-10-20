// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package clusteragentunreachable

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent/connectivity"
)

type checker struct {
	tracker  *connectivity.Tracker
	hostname hostnameinterface.Component
}

func newChecker(tracker *connectivity.Tracker, hostname hostnameinterface.Component) *checker {
	return &checker{tracker: tracker, hostname: hostname}
}

// Check reports an issue once the Cluster Agent has been unreachable for at least failureThreshold.
func (c *checker) Check() ([]runnerdef.IssueReport, error) {
	failure, failing := c.tracker.FailingFor(failureThreshold)
	if !failing {
		return nil, nil
	}

	var hostname string
	if c.hostname != nil {
		hostname = c.hostname.GetSafe(context.Background())
	}

	return []runnerdef.IssueReport{{
		IssueID:   hostIssueID(hostname),
		IssueName: IssueName,
		Context: map[string]string{
			contextKeyError:        failure.LastError,
			contextKeyFailingSince: failure.Since.UTC().Format(time.RFC3339),
			contextKeyHostname:     hostname,
		},
	}}, nil
}

// hostIssueID scopes the id per host: the backend dedups on id alone, and a
// node that can't reach the Cluster Agent is often a node-local network problem.
func hostIssueID(hostname string) string {
	h := fnv.New64a()
	h.Write([]byte(hostname)) // never returns an error for hash.Hash
	return fmt.Sprintf("%s:%016x", IssueID, h.Sum64())
}
