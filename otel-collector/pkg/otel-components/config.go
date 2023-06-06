package otelcomponents

import (
	"reflect"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
)

type OtelConfig[cfg any] struct {
	config   cfg
	fieldMap map[string]any
	ty       reflect.Type
	v        reflect.Value
}

var _ config.Component = NewOtelConfig[string]("")

func NewOtelConfig[cfg any](conf cfg) *OtelConfig[cfg] {
	ty := reflect.TypeOf(conf)
	v := reflect.ValueOf(conf)
	fieldMap := make(map[string]any)

	for i := 0; i < ty.NumField(); i++ {
		field := ty.Field(i)
		val := v.Field(i)
		fieldMap[field.Name] = val
	}

	c := &OtelConfig[cfg]{
		config:   conf,
		fieldMap: fieldMap,
		ty:       ty,
		v:        v,
	}
	return c
}

func (c *OtelConfig[cfg]) Get(key string) interface{} {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetString(key string) string {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetBool(key string) bool {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetInt(key string) int {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetInt32(key string) int32 {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetInt64(key string) int64 {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetFloat64(key string) float64 {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetTime(key string) time.Time {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetDuration(key string) time.Duration {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetStringSlice(key string) []string {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetFloat64SliceE(key string) ([]float64, error) {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetStringMap(key string) map[string]interface{} {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetStringMapString(key string) map[string]string {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetStringMapStringSlice(key string) map[string][]string {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetSizeInBytes(key string) uint {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) GetProxies() *pkgconfig.Proxy {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) ConfigFileUsed() string {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) AllSettings() map[string]interface{} {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) AllSettingsWithoutDefault() map[string]interface{} {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) AllKeys() []string {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) IsSet(key string) bool {
	panic("not implemented") // TODO: Implement
}

// IsKnown returns whether this key is known
func (c *OtelConfig[cfg]) IsKnown(key string) bool {
	panic("not implemented") // TODO: Implement
}

// GetKnownKeys returns all the keys that meet at least one of these criteria:
// 1) have a default, 2) have an environment variable binded, 3) are an alias or 4) have been SetKnown()
func (c *OtelConfig[cfg]) GetKnownKeys() map[string]interface{} {
	panic("not implemented") // TODO: Implement
}

// GetEnvVars returns a list of the env vars that the config supports.
// These have had the EnvPrefix applied, as well as the EnvKeyReplacer.
func (c *OtelConfig[cfg]) GetEnvVars() []string {
	panic("not implemented") // TODO: Implement
}

// IsSectionSet checks if a given section is set by checking if any of
// its subkeys is set.
func (c *OtelConfig[cfg]) IsSectionSet(section string) bool {
	panic("not implemented") // TODO: Implement
}

func (c *OtelConfig[cfg]) Warnings() *pkgconfig.Warnings {
	panic("not implemented") // TODO: Implement
}
