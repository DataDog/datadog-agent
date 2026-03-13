// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package discovery

import (
	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// matchingProcessServiceDiscoveryData checks that the given processes contain at least 1 process with the expected service discovery data
// we cannot fail fast because many processes with the same name are not instrumented with service discovery data
func matchingProcessServiceDiscoveryData(procs []*agentmodel.Process, expectedLanguage agentmodel.Language, expectedPortInfo *agentmodel.PortInfo, expectedServiceDiscovery *agentmodel.ServiceDiscovery) bool {
	for _, proc := range procs {
		// check language
		if proc.Language != expectedLanguage {
			continue
		}

		// check port info
		if !matchingPortInfo(expectedPortInfo, proc.PortInfo) {
			continue
		}

		// check service discovery
		if expectedServiceDiscovery.ApmInstrumentation != proc.ServiceDiscovery.ApmInstrumentation {
			continue
		}

		if !matchingServiceName(expectedServiceDiscovery.DdServiceName, proc.ServiceDiscovery.DdServiceName) {
			continue
		}

		if !matchingServiceName(expectedServiceDiscovery.GeneratedServiceName, proc.ServiceDiscovery.GeneratedServiceName) {
			continue
		}

		if !matchingTracerMetadata(expectedServiceDiscovery.TracerMetadata, proc.ServiceDiscovery.TracerMetadata) {
			continue
		}

		if !matchingServiceNames(expectedServiceDiscovery.AdditionalGeneratedNames, proc.ServiceDiscovery.AdditionalGeneratedNames) {
			continue
		}
		return true
	}
	return false
}

func matchingServiceName(a, b *agentmodel.ServiceName) bool {
	return matchingServiceNames([]*agentmodel.ServiceName{a}, []*agentmodel.ServiceName{b})
}

func matchingServiceNames(expectedServiceNames []*agentmodel.ServiceName, actualServiceNames []*agentmodel.ServiceName) bool {
	// Sort by ServiceName so order doesn't matter
	sort := cmpopts.SortSlices(func(a, b *agentmodel.ServiceName) bool {
		// handles cases where ServiceName is the same
		return a.Name != b.Name && a.Name < b.Name ||
			a.Source < b.Source
	})
	diff := cmp.Diff(expectedServiceNames, actualServiceNames, cmpopts.EquateEmpty(), sort)
	return diff == ""
}

func matchingPortInfo(expectedPortInfo *agentmodel.PortInfo, actualPortInfo *agentmodel.PortInfo) bool {
	if expectedPortInfo == nil {
		return actualPortInfo == nil
	} else if actualPortInfo == nil {
		// expectedPortInfo is not nil so actualPortInfo should not be
		return false
	}

	diffTCP := cmp.Diff(expectedPortInfo.Tcp, actualPortInfo.Tcp, cmpopts.EquateEmpty(), cmpopts.SortSlices(func(a, b int32) bool { return a < b }))
	diffUDP := cmp.Diff(expectedPortInfo.Udp, actualPortInfo.Udp, cmpopts.EquateEmpty(), cmpopts.SortSlices(func(a, b int32) bool { return a < b }))
	return diffTCP == "" && diffUDP == ""
}

func matchingTracerMetadata(expectedTracerMetadata []*agentmodel.TracerMetadata, actualTracerMetadata []*agentmodel.TracerMetadata) bool {
	// tracer metadata contains a uuid (TracerMetadata.RuntimeID), so we ignore it
	// Sort by ServiceName so order doesn't matter
	sortByName := cmpopts.SortSlices(func(a, b *agentmodel.TracerMetadata) bool {
		// handles cases where ServiceName is the same
		return a.ServiceName != b.ServiceName && a.ServiceName < b.ServiceName ||
			a.RuntimeId < b.RuntimeId
	})

	// Ignore RuntimeID field completely
	ignoreID := cmpopts.IgnoreFields(agentmodel.TracerMetadata{}, "RuntimeId")

	diff := cmp.Diff(expectedTracerMetadata, actualTracerMetadata, cmpopts.EquateEmpty(), ignoreID, sortByName)
	return diff == ""
}
