// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agonsticapi

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// init adds the shared library loader to the scheduler
func init() {
	factory := func(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component) (check.Loader, int, error) {
		loader, err := newLoader(senderManager, logReceiver, tagger)
		priority := 40
		return loader, priority, err
	}

	loaders.RegisterLoader(factory)
}
