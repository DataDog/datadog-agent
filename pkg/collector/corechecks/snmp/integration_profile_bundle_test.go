// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package snmp

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/noopimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
)

func TestProfileBundleJsonZip(t *testing.T) {
	timeNow = common.MockTimeNow
	aggregator.NewBufferedAggregator(nil, nil, nooptagger.NewTaggerClient(), "", 1*time.Hour)
	invalidPath, _ := filepath.Abs(filepath.Join("internal", "test", "zipprofiles.d"))
	pkgconfigsetup.Datadog().SetWithoutSource("confd_path", invalidPath)

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	chk := Check{sessionFactory: sessionFactory}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
profile: profile-from-ui
oid_batch_size: 20
namespace: profile-metadata
collect_topology: false
`)
	// language=yaml
	rawInitConfig := []byte(``)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, rawInitConfig, "test")
	assert.NoError(t, err)

	assert.Contains(t, chk.config.Profiles, "profile-from-ui")
}
