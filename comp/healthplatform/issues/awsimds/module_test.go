// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package awsimds

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

func testModule(hostname string) *awsIMDSModule {
	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname(hostname))
	return NewModule(issues.ModuleDeps{Hostname: hn, SelfIdent: selfident.New(nil)}).(*awsIMDSModule)
}

func TestInstanceIssueID(t *testing.T) {
	m := testModule("host-a")
	id := m.instanceIssueID()
	assert.True(t, strings.HasPrefix(id, IssueID+":"))
	assert.Len(t, id, len(IssueID)+1+16)
	assert.Equal(t, id, testModule("host-a").instanceIssueID())
	assert.NotEqual(t, id, testModule("host-b").instanceIssueID())
}
