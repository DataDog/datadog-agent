// Package hostinfo wraps the hostinfo inside a component. This is useful because it is relied on by other components.
package hostinfo

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type Component interface {
	Object() *checks.HostInfo
}

var Module = fxutil.Component(
	fx.Provide(newHostInfo),
)
