package cache

import (
	"go.uber.org/fx"
)

// Module for testing components that need KeyedStringInterners.
var Module = fx.Module("cache", fx.Provide(NewKeyedStringInterner))
