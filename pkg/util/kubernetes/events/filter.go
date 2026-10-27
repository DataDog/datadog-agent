// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import "slices"

// CollectedType selects the Kubernetes events to collect. An event matches when it
// matches every field that is set: the kind of the involved object, the source
// component, and one of the reasons (any reason when none is listed).
type CollectedType struct {
	Kind    string   `yaml:"kind"`
	Source  string   `yaml:"source"`
	Reasons []string `yaml:"reasons"`
}

var integrationToCollectedEventTypes = map[string][]CollectedType{
	"kubernetes": {
		{
			Kind:    "Pod",
			Reasons: []string{"Failed", "BackOff", "Unhealthy", "FailedScheduling", "FailedMount", "FailedAttachVolume"},
		},
		{
			Kind:    "Node",
			Reasons: []string{"TerminatingEvictedPod", "NodeNotReady", "Rebooted", "HostPortConflict"},
		},
		{
			Kind:    "CronJob",
			Reasons: []string{"SawCompletedJob"},
		},
	},
	"kube_scheduler": {
		{
			Kind:    "Pod",
			Reasons: []string{"Failed", "BackOff", "Unhealthy", "FailedScheduling", "FailedMount", "FailedAttachVolume"},
		},
		{
			Kind:    "Node",
			Reasons: []string{"TerminatingEvictedPod", "NodeNotReady", "Rebooted", "HostPortConflict"},
		},
		{
			Kind:    "CronJob",
			Reasons: []string{"SawCompletedJob"},
		},
	},
	"kubernetes controller manager": {
		{
			Kind:    "Pod",
			Reasons: []string{"Failed", "BackOff", "Unhealthy", "FailedScheduling", "FailedMount", "FailedAttachVolume"},
		},
		{
			Kind:    "Node",
			Reasons: []string{"TerminatingEvictedPod", "NodeNotReady", "Rebooted", "HostPortConflict"},
		},
		{
			Kind:    "CronJob",
			Reasons: []string{"SawCompletedJob"},
		},
	},
	"karpenter": {
		{
			Source: "karpenter",
			Reasons: []string{
				"DisruptionBlocked",
				"DisruptionLaunching",
				"DisruptionTerminating",
				"DisruptionWaitingReadiness",
				"FailedDraining",
				"InstanceTerminating",
				"SpotInterrupted",
				"SpotRebalanceRecommendation",
				"TerminatingOnInterruption",
			},
		},
	},
	"datadog-operator": {
		{
			Source: "datadog-operator",
		},
	},
	"amazon elb": {
		{
			Source: "amazon elb",
		},
	},
	"cilium": {
		{
			Source: "cilium",
		},
	},
	"fluxcd": {
		{
			Source: "fluxcd",
		},
	},
	"kubernetes cluster autoscaler": {
		{

			Source: "kubernetes cluster autoscaler",
		},
	},
	"spark": {
		{
			Source: "spark",
		},
	},
	"vault": {

		{
			Source: "vault",
		},
	},
	"default": {
		{
			Reasons: []string{"BackOff"}, // Change tracking consumes all CLB events
		},
	},
}

// ShouldCollectByDefault returns whether ev is collected by the default collected event
// types of the integration it is attributed to (see SourceType).
func ShouldCollectByDefault(ev Event, detectionEnabled bool) bool {
	source := SourceType(ev.ReportingController, ev.SourceComponent, detectionEnabled)
	if v, ok := integrationToCollectedEventTypes[source]; ok {
		return ShouldCollect(ev, append(v, integrationToCollectedEventTypes["default"]...))
	}
	return ShouldCollect(ev, integrationToCollectedEventTypes["default"])
}

// ShouldCollect returns whether ev matches at least one of collectedTypes.
func ShouldCollect(ev Event, collectedTypes []CollectedType) bool {
	for _, f := range collectedTypes {
		if f.Kind != "" && f.Kind != ev.InvolvedObject.Kind {
			continue
		}

		if f.Source != "" && f.Source != ev.SourceComponent {
			continue
		}

		if len(f.Reasons) == 0 {
			return true
		}

		if slices.Contains(f.Reasons, ev.Reason) {
			return true
		}
	}

	return false
}
