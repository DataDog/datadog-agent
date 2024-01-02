// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package demultiplexerimpl

import (
	"testing"

	demultiplexerComp "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/require"
)

func TestSetDefaultSender(t *testing.T) {
	mock := fxutil.Test[demultiplexerComp.Mock](t, MockModule(),
		core.MockBundle(),
		defaultforwarder.MockModule())

	sender := &mocksender.MockSender{}
	mock.SetDefaultSender(sender)

	var component demultiplexerComp.Component = mock

	lazySenderManager, err := component.LazyGetSenderManager()
	require.NoError(t, err)

	componentSender, err := lazySenderManager.GetDefaultSender()
	require.NoError(t, err)
	require.Equal(t, sender, componentSender)

	componentSender, err = component.GetDefaultSender()
	require.NoError(t, err)
	require.Equal(t, sender, componentSender)
}
