// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package main

// stream identifies which OpenTelemetry Collector repository publishes a module.
// It selects which pair of release versions (stable/unstable) applies, and is
// stored explicitly rather than inferred because the stable and unstable numbers
// of the two repos can coincide (see ddotVersion).
const (
	streamCore    = "core"
	streamContrib = "contrib"
)

// component describes how a single OTel module is versioned, independently of any
// release.
//
//   - Stream is "core" or "contrib".
//   - FirstStable is the first v1.x release at which the module graduated to its
//     stable stream, taken from the module's upstream version history. An empty
//     FirstStable means the module has never had a stable stream, so it is always
//     pinned to the unstable (v0.x) version.
//
// For a requested release, newVersionFor picks the stable version when
// FirstStable is set and the release's stable version is >= FirstStable, and the
// unstable version otherwise (including when an older release is requested, from
// before the module became stable).
type component struct {
	Stream      string
	FirstStable string
}

// components is the static, hardcoded catalog that replaces the former external
// lookup. Keyed by module path. A module absent from this map is treated as
// unknown (Datadog or third-party) and left untouched by the tool.
//
// It is seeded from the modules currently pinned in
// comp/otelcol/collector-contrib/impl/manifest.yaml, with FirstStable values taken
// from each module's upstream release history (Go module proxy). As of this
// writing only the confmap providers have graduated to a stable (v1.x) stream;
// every other core/contrib module here still publishes only v0.x. Extend this map
// with the same methodology to cover core/contrib modules beyond the manifest.
var components = map[string]component{
	// --- core: confmap providers (stable, v1.x) ---
	// FirstStable is the module's first v1.x tag from the upstream release history
	// (Go module proxy). OTel graduates stable modules at the current release-train
	// minor, not at v1.0.0, so these are v1.15.0 / v1.17.0.
	"go.opentelemetry.io/collector/confmap/provider/envprovider":   {Stream: streamCore, FirstStable: "v1.15.0"},
	"go.opentelemetry.io/collector/confmap/provider/fileprovider":  {Stream: streamCore, FirstStable: "v1.15.0"},
	"go.opentelemetry.io/collector/confmap/provider/httpprovider":  {Stream: streamCore, FirstStable: "v1.17.0"},
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider": {Stream: streamCore, FirstStable: "v1.17.0"},
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider":  {Stream: streamCore, FirstStable: "v1.17.0"},

	// --- core: still on the unstable (v0.x) stream ---
	"go.opentelemetry.io/collector/receiver/nopreceiver":             {Stream: streamCore},
	"go.opentelemetry.io/collector/receiver/otlpreceiver":            {Stream: streamCore},
	"go.opentelemetry.io/collector/processor/batchprocessor":         {Stream: streamCore},
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor": {Stream: streamCore},
	"go.opentelemetry.io/collector/exporter/debugexporter":           {Stream: streamCore},
	"go.opentelemetry.io/collector/exporter/nopexporter":             {Stream: streamCore},
	"go.opentelemetry.io/collector/exporter/otlpexporter":            {Stream: streamCore},
	"go.opentelemetry.io/collector/exporter/otlphttpexporter":        {Stream: streamCore},
	"go.opentelemetry.io/collector/extension/zpagesextension":        {Stream: streamCore},

	// --- contrib: all currently on the unstable (v0.x) stream ---
	"github.com/open-telemetry/opentelemetry-collector-contrib/connector/countconnector":                {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/connector/routingconnector":              {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector":          {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/loadbalancingexporter":          {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension":          {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension":                {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/dockerobserver":       {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/ecsobserver":          {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/hostobserver":         {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/k8sobserver":          {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/k8sleaderelector":              {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogextension":              {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/filestorage":           {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor":           {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/cumulativetodeltaprocessor":    {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/deltatorateprocessor":          {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/filterprocessor":               {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/groupbyattrsprocessor":         {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/k8sattributesprocessor":        {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/probabilisticsamplerprocessor": {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor":    {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor":             {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor":         {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor":            {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/dockerstatsreceiver":            {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver":                {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/fluentforwardreceiver":          {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver":            {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/httpcheckreceiver":              {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jaegerreceiver":                 {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/k8sclusterreceiver":             {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/k8seventsreceiver":              {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/k8sobjectsreceiver":             {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkareceiver":                  {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kubeletstatsreceiver":           {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/podmanreceiver":                 {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver":             {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/receivercreator":                {Stream: streamContrib},
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver":                 {Stream: streamContrib},
}
