// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package issues

import (
	"os"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
)

// IssueDiscriminator returns the identifier issue ids should be scoped by.
//
// selfIdent wins when it resolves something: every agent owned by the same
// Kubernetes DaemonSet reports the same uid, so identical failures caused by one
// cluster-distributed template compute the same issue id and the backend
// collapses them into a single issue instead of one per host.
//
// It resolves nothing on agents that are not part of a DaemonSet, and on flavors
// built without Kubernetes support where selfident is a no-op. Scoping then
// falls back to hostID — or the OS hostname if the caller had no host id — which
// keeps issue ids distinct per host. Returning "" is a last resort: an empty
// discriminator would make every agent in the org compute the same issue id for
// the same config path, which is exactly the aggregation collapse this scoping
// exists to prevent (see the package doc).
//
// selfIdent may be nil: ModuleDeps is a plain struct, so a caller that leaves
// the field unset (tests do) would otherwise panic only on Kubernetes builds —
// the no-op SelfIdent never dereferences its receiver, the real one does.
func IssueDiscriminator(selfIdent *selfident.SelfIdent, hostID string) string {
	if selfIdent != nil {
		if discriminator := selfIdent.IssueDiscriminator(); discriminator != "" {
			return discriminator
		}
	}
	if hostID != "" {
		return hostID
	}
	if osHostname, err := os.Hostname(); err == nil {
		return osHostname
	}
	return ""
}
