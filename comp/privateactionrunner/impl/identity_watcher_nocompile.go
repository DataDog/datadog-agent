// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !kubeapiserver

package privateactionrunnerimpl

import "context"

// startIdentityWatcher is a no-op in builds without the kubeapiserver tag.
// Hot-reload of PAR credentials is only supported in the cluster agent.
func (p *PrivateActionRunner) startIdentityWatcher(_ context.Context) {}
