// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"testing"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	collectoraggregator "github.com/DataDog/datadog-agent/pkg/collector/aggregator"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/assert"
)

import "C"

func testIsContainerExcluded(t *testing.T) {
	sender := mocksender.NewMockSender("testID")
	logReceiver := option.None[integrations.Component]()
	tagger := nooptagger.NewComponent()

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("container_exclude", []string{"image:bar", "kube_namespace:black"})
	mockConfig.SetWithoutSource("container_include", "kube_namespace:white")
	filterStore := workloadfilterfxmock.SetupMockFilter(t)
	collectoraggregator.ScopeInitCheckContext(sender.GetSenderManager(), logReceiver, tagger, filterStore)

	assert.Equal(t, C.int(1), IsContainerExcluded(C.CString("foo"), C.CString("bar"), C.CString("ns")))
	assert.Equal(t, C.int(0), IsContainerExcluded(C.CString("foo"), C.CString("bar"), C.CString("white")))
	assert.Equal(t, C.int(1), IsContainerExcluded(C.CString("foo"), C.CString("baz"), C.CString("black")))
	assert.Equal(t, C.int(0), IsContainerExcluded(C.CString("foo"), C.CString("baz"), nil))
}
