package newstylefx

import (
	newstyleimpl "github.com/DataDog/datadog-agent/comp/newstyle/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			newstyleimpl.NewComponent,
		),
	)
}
