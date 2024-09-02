package compressionimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewCompressor),
	)
}

func NewCompressor(cfg config.Component) compression.Component {
	return GetCompressor(
		cfg.GetString("serializer_compressor_kind"),
		cfg.GetInt("serializer_zstd_compressor_level"),
	)
}
