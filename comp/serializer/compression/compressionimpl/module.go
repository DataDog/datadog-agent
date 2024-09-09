package compressionimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl/strategy"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewCompressorFactory),
	)
}

type CompressorFactory struct{}

func NewCompressorFactory() compression.Factory {
	return &CompressorFactory{}
}

func FromConfig(cfg config.Component) compression.Component {
	return NewCompressorFactory().NewCompressor(
		cfg.GetString("serializer_compressor_kind"),
		cfg.GetInt("serializer_zstd_compressor_level"),
		"serializer_compressor_kind",
		[]string{"zstd", "zlib"},
	)
}

func (_ *CompressorFactory) NewNoopCompressor() compression.Component {
	return strategy.NewNoopStrategy()
}
