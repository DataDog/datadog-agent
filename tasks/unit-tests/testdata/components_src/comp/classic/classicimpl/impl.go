package classicimpl

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In
}

type provides struct {
	fx.Out
}

type implementation struct{}

func newClassic() Component {
	return &implementation{}
}

func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newClassic))
}
