// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentstackmonitorimpl

import (
	"fmt"
	"maps"
	"strconv"
	"strings"

	issuetemplates "github.com/DataDog/datadog-agent/comp/healthplatform/issues/agentstackmonitor"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// Each threshold acts as its own minimum-evidence bar; we don't wait for the
// buffer to fill. bufferSize only bounds how far back history is retained.
const (
	memRatioThreshold        = 0.9
	memPressureMinSamples    = 8
	restartDeltaMinSum       = 3 // "delta > 2"
	crashLoopMinObservations = 3
)

// evaluate returns at most one report per rule per tick. The healthplatform
// runner handles create/resolve diffing by IssueID.
func evaluate(s *subjectState) []runnerdef.IssueReport {
	if s == nil {
		return nil
	}
	var reports []runnerdef.IssueReport
	ctx := baseContext(s)

	if r, ok := evaluateMemoryPressure(s, ctx); ok {
		reports = append(reports, r)
	}
	if r, ok := evaluateContainerRestart(s, ctx); ok {
		reports = append(reports, r)
	}
	if r, ok := evaluateOOMKilled(s, ctx); ok {
		reports = append(reports, r)
	}
	if r, ok := evaluateCrashLoopBackOff(s, ctx); ok {
		reports = append(reports, r)
	}
	return reports
}

func baseContext(s *subjectState) map[string]string {
	return map[string]string{
		issuetemplates.CtxSubjectKind:    string(s.subjectKind),
		issuetemplates.CtxNamespace:      s.controller.Namespace,
		issuetemplates.CtxControllerKind: s.controller.Kind,
		issuetemplates.CtxControllerName: s.controller.Name,
		issuetemplates.CtxContainerName:  s.containerName,
		issuetemplates.CtxPodName:        s.podName,
		issuetemplates.CtxPodUID:         s.podUID,
	}
}

// issueID is controller-anchored so reports survive pod reschedules.
func issueID(slug string, s *subjectState) string {
	return fmt.Sprintf("%s:%s/%s/%s:%s",
		slug, s.controller.Namespace, s.controller.Kind, s.controller.Name, s.containerName)
}

func evaluateMemoryPressure(s *subjectState, base map[string]string) (runnerdef.IssueReport, bool) {
	over := s.memRatio.countMatching(func(v float64) bool { return v > memRatioThreshold })
	if over < memPressureMinSamples {
		return runnerdef.IssueReport{}, false
	}
	ctx := copyMap(base)
	ctx[issuetemplates.CtxMemoryUsage] = strconv.FormatFloat(s.memUsage, 'f', 0, 64)
	ctx[issuetemplates.CtxMemoryLimit] = strconv.FormatFloat(s.memLimit, 'f', 0, 64)
	if s.memLimit > 0 {
		ctx[issuetemplates.CtxMemoryRatio] = strconv.FormatFloat(s.memUsage/s.memLimit, 'f', 3, 64)
	}
	ctx[issuetemplates.CtxSamplesOverThresh] = strconv.Itoa(over)
	return runnerdef.IssueReport{
		IssueID:   issueID("agentstackmonitor.memory-pressure", s),
		IssueName: issuetemplates.IssueNameMemoryPressure,
		Source:    issuetemplates.Source,
		Context:   ctx,
	}, true
}

func evaluateContainerRestart(s *subjectState, base map[string]string) (runnerdef.IssueReport, bool) {
	total := sumInt(&s.restartDeltas)
	if total < restartDeltaMinSum {
		return runnerdef.IssueReport{}, false
	}
	ctx := copyMap(base)
	ctx[issuetemplates.CtxRestartsInWindow] = strconv.Itoa(int(total))
	ctx[issuetemplates.CtxLastTermReason] = s.lastTermReason
	return runnerdef.IssueReport{
		IssueID:   issueID("agentstackmonitor.container-restart", s),
		IssueName: issuetemplates.IssueNameContainerRestart,
		Source:    issuetemplates.Source,
		Context:   ctx,
	}, true
}

// evaluateOOMKilled requires a restart within the window so stale
// LastTerminationState (kubelet keeps it forever) doesn't re-fire.
func evaluateOOMKilled(s *subjectState, base map[string]string) (runnerdef.IssueReport, bool) {
	if !strings.EqualFold(s.lastTermReason, "OOMKilled") {
		return runnerdef.IssueReport{}, false
	}
	if sumInt(&s.restartDeltas) == 0 {
		return runnerdef.IssueReport{}, false
	}
	ctx := copyMap(base)
	ctx[issuetemplates.CtxLastTermReason] = s.lastTermReason
	ctx[issuetemplates.CtxMemoryLimit] = strconv.FormatFloat(s.memLimit, 'f', 0, 64)
	return runnerdef.IssueReport{
		IssueID:   issueID("agentstackmonitor.oomkilled", s),
		IssueName: issuetemplates.IssueNameContainerOOMKilled,
		Source:    issuetemplates.Source,
		Context:   ctx,
	}, true
}

func evaluateCrashLoopBackOff(s *subjectState, base map[string]string) (runnerdef.IssueReport, bool) {
	observed := s.waitingReason.countMatching(func(r string) bool { return r == "CrashLoopBackOff" })
	if observed < crashLoopMinObservations {
		return runnerdef.IssueReport{}, false
	}
	ctx := copyMap(base)
	ctx[issuetemplates.CtxWaitingObservedIn] = strconv.Itoa(observed)
	ctx[issuetemplates.CtxLastTermReason] = s.lastTermReason
	return runnerdef.IssueReport{
		IssueID:   issueID("agentstackmonitor.crashloopbackoff", s),
		IssueName: issuetemplates.IssueNameCrashLoopBackOff,
		Source:    issuetemplates.Source,
		Context:   ctx,
	}, true
}

func copyMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}
