// Package fxgzip provides fx options for the compression component.
package fxgzip

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	implgzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
)

// Module specifies the compression module.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			implgzip.NewComponent,
		),
	)
}
