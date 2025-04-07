// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package mock

import (
	compdef "github.com/DataDog/datadog-agent/comp/def"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	noneimpl "github.com/DataDog/datadog-agent/comp/logs/auditor/impl-none"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// ProvidesMock is the mock component output
type ProvidesMock struct {
	compdef.Out

	Comp auditor.Component
}

// AuditorMockModule defines the fx options for the mock component.
func AuditorMockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newMock),
	)
}

func newMock() ProvidesMock {
	return ProvidesMock{
		Comp: noneimpl.NewAuditor(),
	}
}
