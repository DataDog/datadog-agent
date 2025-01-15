package selector

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	common "github.com/DataDog/datadog-agent/pkg/util/compression"
)

// FromConfig will return the compression algorithm specified in the provided config
// under the `serializer_compressor_kind` key.
// If `zstd` the compression level is taken from the serializer_zstd_compressor_level
// key.
func FromConfig(cfg config.Reader) Compressor {
	kind := cfg.GetString("serializer_compressor_kind")
	var level int

	switch kind {
	case common.ZstdKind:
		level = cfg.GetInt("serializer_zstd_compressor_level")
	case common.GzipKind:
		// There is no configuration option for gzip compression level when set via this method.
		level = 6
	}

	return NewCompressor(kind, level)
}
