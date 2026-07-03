// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentstackmonitor

import (
	"fmt"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// Context keys populated by the detection component and consumed by BuildIssue.
const (
	CtxSubjectKind       = "subject_kind"
	CtxNamespace         = "kube_namespace"
	CtxControllerKind    = "controller_kind"
	CtxControllerName    = "controller_name"
	CtxContainerName     = "container_name"
	CtxPodName           = "pod_name"
	CtxPodUID            = "pod_uid"
	CtxMemoryUsage       = "memory_usage_bytes"
	CtxMemoryLimit       = "memory_limit_bytes"
	CtxMemoryRatio       = "memory_usage_ratio"
	CtxSamplesOverThresh = "samples_over_threshold"
	CtxRestartsInWindow  = "restarts_in_window"
	CtxLastTermReason    = "last_termination_reason"
	CtxLastExitCode      = "last_exit_code"
	CtxWaitingObservedIn = "waiting_observed_in_window"
	CtxPodReason         = "pod_reason"
)

func location(ctx map[string]string) string {
	return fmt.Sprintf("%s/%s/%s", ctx[CtxNamespace], ctx[CtxControllerKind], ctx[CtxControllerName])
}

func tagList(ctx map[string]string) []string {
	tags := []string{
		"agent_health",
		"subject_kind:" + ctx[CtxSubjectKind],
		"kube_namespace:" + ctx[CtxNamespace],
	}
	if v := ctx[CtxContainerName]; v != "" {
		tags = append(tags, "container_name:"+v)
	}
	if v := ctx[CtxPodName]; v != "" {
		tags = append(tags, "pod_name:"+v)
	}
	return tags
}

func extraStruct(ctx map[string]string, extras map[string]any) *structpb.Struct {
	base := map[string]any{
		"pod_uid":         ctx[CtxPodUID],
		"pod_name":        ctx[CtxPodName],
		"kube_namespace":  ctx[CtxNamespace],
		"controller_kind": ctx[CtxControllerKind],
		"controller_name": ctx[CtxControllerName],
		"container_name":  ctx[CtxContainerName],
	}
	for k, v := range extras {
		base[k] = v
	}
	s, _ := structpb.NewStruct(base)
	return s
}

func buildMemoryPressureIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	title := fmt.Sprintf("Sustained memory pressure on %s/%s (%s)",
		ctx[CtxControllerName], ctx[CtxContainerName], ctx[CtxSubjectKind])
	desc := fmt.Sprintf(
		"%s of the last 10 samples showed memory usage above 90%% of the container limit (last reading: %s / %s bytes) for container %s in %s/%s.",
		ctx[CtxSamplesOverThresh], ctx[CtxMemoryUsage], ctx[CtxMemoryLimit],
		ctx[CtxContainerName], ctx[CtxNamespace], ctx[CtxControllerName])
	return &healthplatform.Issue{
		IssueName:   IssueNameMemoryPressure,
		Title:       title,
		Description: desc,
		Category:    "agent-self-health",
		Location:    location(ctx),
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
		Source:      Source,
		Extra: extraStruct(ctx, map[string]any{
			"memory_usage_bytes":     ctx[CtxMemoryUsage],
			"memory_limit_bytes":     ctx[CtxMemoryLimit],
			"memory_usage_ratio":     ctx[CtxMemoryRatio],
			"samples_over_threshold": ctx[CtxSamplesOverThresh],
		}),
		Tags: append(tagList(ctx), "condition:memory_pressure"),
	}, nil
}

func buildContainerRestartIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	title := fmt.Sprintf("Container restarting repeatedly (%s/%s in %s)",
		ctx[CtxControllerName], ctx[CtxContainerName], ctx[CtxSubjectKind])
	desc := fmt.Sprintf(
		"Container %s in %s/%s restarted %s times over the last 10 minutes (most recent termination reason: %s).",
		ctx[CtxContainerName], ctx[CtxNamespace], ctx[CtxControllerName],
		ctx[CtxRestartsInWindow], displayReason(ctx[CtxLastTermReason]))
	return &healthplatform.Issue{
		IssueName:   IssueNameContainerRestart,
		Title:       title,
		Description: desc,
		Category:    "agent-self-health",
		Location:    location(ctx),
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		Source:      Source,
		Extra: extraStruct(ctx, map[string]any{
			"restarts_in_window":      ctx[CtxRestartsInWindow],
			"last_termination_reason": ctx[CtxLastTermReason],
			"last_exit_code":          ctx[CtxLastExitCode],
		}),
		Tags: appendReasonTags(tagList(ctx), ctx, "condition:container_restart"),
	}, nil
}

func buildContainerOOMKilledIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	title := fmt.Sprintf("Container OOMKilled (%s/%s in %s)",
		ctx[CtxControllerName], ctx[CtxContainerName], ctx[CtxSubjectKind])
	desc := fmt.Sprintf(
		"Container %s in %s/%s was OOMKilled within the observation window (memory limit: %s bytes).",
		ctx[CtxContainerName], ctx[CtxNamespace], ctx[CtxControllerName], ctx[CtxMemoryLimit])
	return &healthplatform.Issue{
		IssueName:   IssueNameContainerOOMKilled,
		Title:       title,
		Description: desc,
		Category:    "agent-self-health",
		Location:    location(ctx),
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
		Source:      Source,
		Extra: extraStruct(ctx, map[string]any{
			"memory_limit_bytes": ctx[CtxMemoryLimit],
			"last_exit_code":     ctx[CtxLastExitCode],
		}),
		Tags: appendReasonTags(tagList(ctx), ctx, "condition:oomkilled", "termination_reason:oomkilled"),
	}, nil
}

func buildCrashLoopBackOffIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	title := fmt.Sprintf("Container in CrashLoopBackOff (%s/%s in %s)",
		ctx[CtxControllerName], ctx[CtxContainerName], ctx[CtxSubjectKind])
	desc := fmt.Sprintf(
		"Container %s in %s/%s has been observed in CrashLoopBackOff for %s of the last 10 checks.",
		ctx[CtxContainerName], ctx[CtxNamespace], ctx[CtxControllerName],
		ctx[CtxWaitingObservedIn])
	return &healthplatform.Issue{
		IssueName:   IssueNameCrashLoopBackOff,
		Title:       title,
		Description: desc,
		Category:    "agent-self-health",
		Location:    location(ctx),
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
		Source:      Source,
		Extra: extraStruct(ctx, map[string]any{
			"crashloop_observations": ctx[CtxWaitingObservedIn],
			"last_exit_code":         ctx[CtxLastExitCode],
		}),
		Tags: appendReasonTags(tagList(ctx), ctx, "condition:crashloopbackoff"),
	}, nil
}

func appendReasonTags(tags []string, ctx map[string]string, extra ...string) []string {
	out := append(tags, extra...)
	if v := ctx[CtxLastTermReason]; v != "" {
		out = append(out, "termination_reason:"+strings.ToLower(v))
	}
	if v := ctx[CtxLastExitCode]; v != "" {
		out = append(out, "exit_code:"+v)
	}
	return out
}

func displayReason(reason string) string {
	if reason == "" {
		return "unknown"
	}
	return reason
}
