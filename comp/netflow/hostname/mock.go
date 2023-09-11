// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package hostname

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockHostname is an alias for injecting a mock hostname.
// Usage: fx.Replace(hostname.MockHostname("whatever"))
type MockHostname string

func newMock(name MockHostname) Component {
	return &hostnameService{string(name)}
}

// MockModule defines the fx options for the mock component.
// Injecting MockModule will provide the hostname 'my-hostname';
// override this with fx.Replace(hostname.MockHostname("whatever")).
var MockModule = fxutil.Component(
	fx.Provide(newMock),
	fx.Supply(MockHostname("my-hostname")),
)
