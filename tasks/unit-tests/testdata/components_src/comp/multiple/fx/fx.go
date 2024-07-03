package multiplefx

import (
	multipleimpl "github.com/DataDog/datadog-agent/comp/multiple/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			multipleimpl.NewComponent,
		),
	)
}
