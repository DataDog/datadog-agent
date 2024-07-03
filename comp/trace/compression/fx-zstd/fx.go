// package fxzstd provides fx options for the compression component.
package fxzstd

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	implzstd "github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd"
)

// Module specifies the compression module.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			implzstd.NewComponent,
		),
	)
}
