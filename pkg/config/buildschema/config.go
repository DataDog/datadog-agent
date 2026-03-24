// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package buildschema

import (
	"io"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

type builder struct {
	Schema map[string]interface{}
}

var _ model.BuildableConfig = (*builder)(nil)
var _ SchemaBuilder = (*builder)(nil)

func NewSchemaBuilder(name, envprefix string, envKeyReplacer *strings.Replacer) model.BuildableConfig {
	return &builder{
		Schema: map[string]interface{}{
			"properties": map[string]interface{}{},
		},
	}
}

type SchemaBuilder interface {
	GetSchema() map[string]interface{}
}

func (b *builder) GetLibType() string {
	return "builder"
}

func (b *builder) GetSchema() map[string]interface{} {
	return b.Schema
}

func (b *builder) SetDefault(key string, value interface{}) {
	b.addToSchema(key, value, nil, true, false)
}

func (b *builder) SetEnvPrefix(in string) {
}

func (b *builder) BindEnv(key string, envvars ...string) {
	b.addToSchema(key, nil, envvars, false, true)
}

func (b *builder) ParseEnvAsStringSlice(key string, fx func(string) []string) {
	// pass
}

func (b *builder) ParseEnvAsMapStringInterface(key string, fx func(string) map[string]interface{}) {
	// pass
}

func (b *builder) ParseEnvAsSliceMapString(key string, fx func(string) []map[string]string) {
	// pass
}

func (b *builder) ParseEnvAsSlice(key string, fx func(string) []interface{}) {
	// pass
}

func (b *builder) SetKnown(key string) {
	b.addToSchema(key, nil, nil, true, true)
}

func (b *builder) BindEnvAndSetDefault(key string, val interface{}, env ...string) {
	b.addToSchema(key, val, env, false, false)
}

func (b *builder) BuildSchema() {
}

// Remaining methods are not implemented and will panic when invoked

func (b *builder) notImplemented() {
	panic("not implemented!")
}

func (b *builder) Get(key string) interface{} {
	b.notImplemented()
	return nil
}

func (b *builder) GetString(key string) string {
	b.notImplemented()
	return ""
}

func (b *builder) GetBool(key string) bool {
	b.notImplemented()
	return false
}

func (b *builder) GetInt(key string) int {
	b.notImplemented()
	return 0
}

func (b *builder) GetInt32(key string) int32 {
	b.notImplemented()
	return 0
}

func (b *builder) GetInt64(key string) int64 {
	b.notImplemented()
	return 0
}

func (b *builder) GetFloat64(key string) float64 {
	b.notImplemented()
	return 0.0
}

func (b *builder) GetDuration(key string) time.Duration {
	b.notImplemented()
	return 0 * time.Second
}

func (b *builder) GetStringSlice(key string) []string {
	b.notImplemented()
	return nil
}

func (b *builder) GetFloat64Slice(key string) []float64 {
	b.notImplemented()
	return nil
}

func (b *builder) GetStringMap(key string) map[string]interface{} {
	b.notImplemented()
	return nil
}

func (b *builder) GetStringMapString(key string) map[string]string {
	b.notImplemented()
	return nil
}

func (b *builder) GetStringMapStringSlice(key string) map[string][]string {
	b.notImplemented()
	return nil
}

func (b *builder) GetSizeInBytes(key string) uint {
	b.notImplemented()
	return 0
}

func (b *builder) GetProxies() *model.Proxy {
	b.notImplemented()
	return nil
}

func (b *builder) GetSequenceID() uint64 {
	b.notImplemented()
	return 0
}

func (b *builder) GetSource(key string) model.Source {
	b.notImplemented()
	return model.SourceUnknown
}

func (b *builder) GetAllSources(key string) []model.ValueWithSource {
	b.notImplemented()
	return nil
}

func (b *builder) GetSubfields(key string) []string {
	b.notImplemented()
	return nil
}

func (b *builder) ConfigFileUsed() string {
	b.notImplemented()
	return ""
}

func (b *builder) ExtraConfigFilesUsed() []string {
	b.notImplemented()
	return nil
}

func (b *builder) AllSettings() map[string]interface{} {
	b.notImplemented()
	return nil
}

func (b *builder) AllSettingsWithoutDefault() map[string]interface{} {
	b.notImplemented()
	return nil
}

func (b *builder) AllSettingsBySource() map[model.Source]interface{} {
	b.notImplemented()
	return nil
}

func (b *builder) AllSettingsWithoutSecrets() map[string]interface{} {
	b.notImplemented()
	return nil
}

func (b *builder) AllSettingsWithoutDefaultOrSecrets() map[string]interface{} {
	b.notImplemented()
	return nil
}

func (b *builder) GetSecretSettingPaths() []string {
	b.notImplemented()
	return nil
}

func (b *builder) AllKeysLowercased() []string {
	b.notImplemented()
	return nil
}

func (b *builder) AllFlattenedSettingsWithSequenceID() (map[string]interface{}, uint64) {
	b.notImplemented()
	return nil, 0
}

func (b *builder) SetTestOnlyDynamicSchema(allow bool) {
	b.notImplemented()
}

func (b *builder) IsSet(key string) bool {
	b.notImplemented()
	return false
}

func (b *builder) IsConfigured(key string) bool {
	b.notImplemented()
	return false
}

func (b *builder) HasSection(key string) bool {
	b.notImplemented()
	return false
}

func (b *builder) IsKnown(key string) bool {
	b.notImplemented()
	return false
}

func (b *builder) GetKnownKeysLowercased() map[string]interface{} {
	b.notImplemented()
	return nil
}

func (b *builder) GetEnvVars() []string {
	b.notImplemented()
	return nil
}

func (b *builder) Warnings() *model.Warnings {
	b.notImplemented()
	return nil
}

func (b *builder) Object() model.Reader {
	b.notImplemented()
	return nil
}

func (b *builder) OnUpdate(callback model.NotificationReceiver) {
	b.notImplemented()
}

func (b *builder) Stringify(source model.Source, opts ...model.StringifyOption) string {
	b.notImplemented()
	return ""
}

func (b *builder) Set(key string, value interface{}, source model.Source) {
	b.notImplemented()
}

func (b *builder) SetWithoutSource(key string, value interface{}) {
	b.notImplemented()
}

func (b *builder) UnsetForSource(key string, source model.Source) {
	b.notImplemented()
}

func (b *builder) SetEnvKeyReplacer(r *strings.Replacer) {
	b.notImplemented()
}

func (b *builder) AddConfigPath(in string) {
	b.notImplemented()
}

func (b *builder) AddExtraConfigPaths(in []string) error {
	b.notImplemented()
	return nil
}

func (b *builder) SetConfigName(in string) {
	b.notImplemented()
}

func (b *builder) SetConfigFile(in string) {
	b.notImplemented()
}

func (b *builder) SetConfigType(in string) {
	b.notImplemented()
}

func (b *builder) ReadInConfig() error {
	b.notImplemented()
	return nil
}

func (b *builder) ReadConfig(in io.Reader) error {
	b.notImplemented()
	return nil
}

func (b *builder) MergeConfig(in io.Reader) error {
	b.notImplemented()
	return nil
}

func (b *builder) MergeFleetPolicy(configPath string) error {
	b.notImplemented()
	return nil
}

func (b *builder) RevertFinishedBackToBuilder() model.BuildableConfig {
	b.notImplemented()
	return nil
}
