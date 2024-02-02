// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"encoding/json"
	"io"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// ContextDebugRepr is the on-disk representation of a context.
type ContextDebugRepr struct {
	Name       string
	Host       string
	Type       string
	TaggerTags []string
	MetricTags []string
	NoIndex    bool
	Source     metrics.MetricSource
}

func (cr *contextResolver) dumpContexts(dest io.Writer) error {
	enc := json.NewEncoder(dest)

	for _, c := range cr.contextsByKey {
		err := enc.Encode(ContextDebugRepr{
			Name:       c.Name,
			Host:       c.Host,
			Type:       c.mtype.String(),
			TaggerTags: c.taggerTags.Tags(),
			MetricTags: c.metricTags.Tags(),
			NoIndex:    c.noIndex,
			Source:     c.source,
		})
		if err != nil {
			return err
		}
	}

	return nil
}
