// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"fmt"

	"go.opentelemetry.io/collector/component"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

// ContainerTagPromotionMode controls whether the processor rewrites or
// duplicates tags from the tagger with a `datadog.container.tag.` prefix so
// that downstream trace-agent / Datadog exporter can promote them into
// `_dd.tags.container` (visible in the Infrastructure tab). Known DD and OTel
// semantic conventions, as well as USM keys, are always exempt — they are
// already promoted through their own paths.
type ContainerTagPromotionMode string

const (
	// ContainerTagPromotionOff keeps the existing behavior: tags from the
	// tagger are written as-is. Only keys recognized by trace-agent (DD or
	// OTel semantic conventions) reach `_dd.tags.container`; custom keys
	// remain plain resource attributes.
	ContainerTagPromotionOff ContainerTagPromotionMode = "off"

	// ContainerTagPromotionDuplicate writes both the original tag and a
	// `datadog.container.tag.<key>` copy. Custom keys reach
	// `_dd.tags.container` via the prefixed copy; the original survives for
	// downstream consumers that read the unprefixed form.
	ContainerTagPromotionDuplicate ContainerTagPromotionMode = "duplicate"

	// ContainerTagPromotionRename writes only the `datadog.container.tag.<key>`
	// form. Smaller resource payload, but downstream consumers that read the
	// unprefixed form lose access to the tag.
	ContainerTagPromotionRename ContainerTagPromotionMode = "rename"
)

// Config defines configuration for processor.
type Config struct {
	// Cardinality controls which tag cardinality is enriched onto the signal.
	// Accepted values: 0 = low (host-level, default), 1 = orchestrator
	// (per-pod/task), 2 = high (per-container/request).
	Cardinality           types.TagCardinality `mapstructure:"cardinality"`
	AllowHostnameOverride bool                 `mapstructure:"allow_hostname_override"`
	// TraceContainerTagPromotion controls how tags emitted by this processor are
	// surfaced for promotion into Datadog container tags. See the
	// ContainerTagPromotionMode constants for the supported values.
	// An empty value is treated as "off".
	//
	// This only affects the traces pipeline: `_dd.tags.container` promotion
	// is a trace-agent-specific mechanism
	// (attributes.ConsumeContainerTagsFromResource), so the logs, metrics,
	// and profiles processors always behave as if this were "off",
	// regardless of the configured value.
	TraceContainerTagPromotion ContainerTagPromotionMode `mapstructure:"trace_container_tag_promotion"`

	// LogsTagsAsDDTags controls whether custom tags emitted by the tagger
	// (e.g. via kubernetesResourcesLabelsAsTags / AnnotationsAsTags) are
	// written as a `ddtags` log record attribute -- which the Datadog logs
	// intake turns into real log tags -- instead of as resource attributes,
	// which surface as log attributes.
	//
	// When false (default), behavior is unchanged: these tags remain
	// resource attributes and appear as log attributes. Known DD / OTel
	// semantic conventions and unified service tagging keys are unaffected
	// either way -- they are always kept as resource attributes, since the
	// Datadog logs intake already promotes them into tags on its own.
	//
	// This only affects the logs pipeline.
	LogsTagsAsDDTags bool `mapstructure:"logs_tags_as_ddtags"`
}

var _ component.Config = (*Config)(nil)

// Validate configuration
func (cfg *Config) Validate() error {
	switch cfg.TraceContainerTagPromotion {
	case "", ContainerTagPromotionOff,
		ContainerTagPromotionDuplicate, ContainerTagPromotionRename:
		return nil
	default:
		return fmt.Errorf(
			"invalid trace_container_tag_promotion %q: must be one of %q, %q, %q",
			cfg.TraceContainerTagPromotion,
			ContainerTagPromotionOff,
			ContainerTagPromotionDuplicate,
			ContainerTagPromotionRename,
		)
	}
}
