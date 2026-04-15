// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package buildschema implement the config component to create a schema from it
package buildschema

import (
	"io"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

type builder struct {
	sync.Mutex

	Schema map[string]interface{}
}

var _ model.BuildableConfig = (*builder)(nil)
var _ SchemaBuilder = (*builder)(nil)

func NewSchemaBuilder(_, _ string, _ *strings.Replacer) model.BuildableConfig {
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

func (b *builder) SetEnvPrefix(_ string) {
}

func (b *builder) BindEnv(key string, envvars ...string) {
	b.addToSchema(key, nil, envvars, false, true)
}

func (b *builder) ParseEnvAsStringSlice(_ string, _ func(string) []string) {
	// pass
}

func (b *builder) ParseEnvAsMapStringInterface(_ string, _ func(string) map[string]interface{}) {
	// pass
}

func (b *builder) ParseEnvAsSliceMapString(_ string, _ func(string) []map[string]string) {
	// pass
}

func (b *builder) ParseEnvAsSlice(_ string, _ func(string) []interface{}) {
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

func (b *builder) Get(_ string) interface{} {
	b.notImplemented()
	return nil
}

func (b *builder) GetString(_ string) string {
	b.notImplemented()
	return ""
}

func (b *builder) GetBool(_ string) bool {
	b.notImplemented()
	return false
}

func (b *builder) GetInt(_ string) int {
	b.notImplemented()
	return 0
}

func (b *builder) GetInt32(_ string) int32 {
	b.notImplemented()
	return 0
}

func (b *builder) GetInt64(_ string) int64 {
	b.notImplemented()
	return 0
}

func (b *builder) GetFloat64(_ string) float64 {
	b.notImplemented()
	return 0.0
}

func (b *builder) GetDuration(_ string) time.Duration {
	b.notImplemented()
	return 0 * time.Second
}

func (b *builder) GetStringSlice(_ string) []string {
	b.notImplemented()
	return nil
}

func (b *builder) GetFloat64Slice(_ string) []float64 {
	b.notImplemented()
	return nil
}

func (b *builder) GetStringMap(_ string) map[string]interface{} {
	b.notImplemented()
	return nil
}

func (b *builder) GetStringMapString(_ string) map[string]string {
	b.notImplemented()
	return nil
}

func (b *builder) GetStringMapStringSlice(_ string) map[string][]string {
	b.notImplemented()
	return nil
}

func (b *builder) GetSizeInBytes(_ string) uint {
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

func (b *builder) GetSource(_ string) model.Source {
	b.notImplemented()
	return model.SourceUnknown
}

func (b *builder) GetAllSources(_ string) []model.ValueWithSource {
	b.notImplemented()
	return nil
}

func (b *builder) GetSubfields(_ string) []string {
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

func (b *builder) SetTestOnlyDynamicSchema(_ bool) {
	b.notImplemented()
}

func (b *builder) IsSet(_ string) bool {
	b.notImplemented()
	return false
}

func (b *builder) IsConfigured(_ string) bool {
	b.notImplemented()
	return false
}

func (b *builder) HasSection(_ string) bool {
	b.notImplemented()
	return false
}

func (b *builder) IsKnown(_ string) bool {
	b.notImplemented()
	return false
}

func (b *builder) IsLeafSetting(key string) bool {
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

func (b *builder) OnUpdate(_ model.NotificationReceiver) {
	b.notImplemented()
}

func (b *builder) Stringify(_ model.Source, _ ...model.StringifyOption) string {
	b.notImplemented()
	return ""
}

func (b *builder) Set(_ string, _ interface{}, _ model.Source) {
	b.notImplemented()
}

func (b *builder) SetWithoutSource(_ string, _ interface{}) {
	b.notImplemented()
}

func (b *builder) UnsetForSource(_ string, _ model.Source) {
	b.notImplemented()
}

func (b *builder) SetEnvKeyReplacer(_ *strings.Replacer) {
	b.notImplemented()
}

func (b *builder) AddConfigPath(_ string) {
	b.notImplemented()
}

func (b *builder) AddExtraConfigPaths(_ []string) error {
	b.notImplemented()
	return nil
}

func (b *builder) SetConfigName(_ string) {
	b.notImplemented()
}

func (b *builder) SetConfigFile(_ string) {
	b.notImplemented()
}

func (b *builder) SetConfigType(_ string) {
	b.notImplemented()
}

func (b *builder) ReadInConfig() error {
	b.notImplemented()
	return nil
}

func (b *builder) ReadConfig(_ io.Reader) error {
	b.notImplemented()
	return nil
}

func (b *builder) MergeConfig(_ io.Reader) error {
	b.notImplemented()
	return nil
}

func (b *builder) MergeFleetPolicy(_ string) error {
	b.notImplemented()
	return nil
}

func (b *builder) RevertFinishedBackToBuilder() model.BuildableConfig {
	b.notImplemented()
	return nil
}
