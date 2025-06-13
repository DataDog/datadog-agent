// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type datadogURLConnectivitySuite struct {
	baseDiagnoseSuite
}

func TestDatadogURLConnectivitySuite(t *testing.T) {
	t.Parallel()
	var suite datadogURLConnectivitySuite
	suite.suites = append(suite.suites, []string{
		"Installer HTTP connectivity",
		"Installer OCI connectivity",
		"YUM connectivity",
		"APT connectivity",
	}...)
	e2e.Run(t, &suite, e2e.WithProvisioner(awshost.Provisioner()))
}

func (v *datadogURLConnectivitySuite) TestURLConnectivityDefaultSite() {
	diagnose := getDiagnoseOutput(&v.baseDiagnoseSuite)
	v.AssertOutputNotError(diagnose)

	diagnose = getDiagnoseOutput(&v.baseDiagnoseSuite, agentclient.WithArgs([]string{"--json"}))
	diagnoseJSON := unmarshalDiagnose(diagnose)
	require.NotNil(v.T(), diagnoseJSON)
	assert.Zero(v.T(), diagnoseJSON.Summary.Fail)
	assert.Zero(v.T(), diagnoseJSON.Summary.UnexpectedErr)

	expectedServices := []string{
		"install",
		"yum",
		"apt",
		"keys",
		"process",
		"flare",
	}
	for _, service := range expectedServices {
		assert.Contains(v.T(), diagnose, service)
	}
}

func (v *datadogURLConnectivitySuite) TestURLConnectivityCustomSite() {
	params := agentparams.WithAgentConfig("site: datad0g.com")
	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(params)))

	diagnose := getDiagnoseOutput(&v.baseDiagnoseSuite)
	v.AssertOutputNotError(diagnose)

	diagnose = getDiagnoseOutput(&v.baseDiagnoseSuite, agentclient.WithArgs([]string{"--json"}))
	diagnoseJSON := unmarshalDiagnose(diagnose)
	require.NotNil(v.T(), diagnoseJSON)
	assert.Zero(v.T(), diagnoseJSON.Summary.Fail)
	assert.Zero(v.T(), diagnoseJSON.Summary.UnexpectedErr)

	// Vérifier que les URLs contiennent le site personnalisé
	assert.Contains(v.T(), diagnose, "datad0g.com")
}

func (v *datadogURLConnectivitySuite) TestURLConnectivityInvalidSite() {
	params := agentparams.WithAgentConfig("site: invalid")
	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(params)))

	diagnose := getDiagnoseOutput(&v.baseDiagnoseSuite)
	v.AssertOutputNotError(diagnose)

	diagnose = getDiagnoseOutput(&v.baseDiagnoseSuite, agentclient.WithArgs([]string{"--json"}))
	diagnoseJSON := unmarshalDiagnose(diagnose)
	require.NotNil(v.T(), diagnoseJSON)
	assert.NotZero(v.T(), diagnoseJSON.Summary.Fail)
	assert.Zero(v.T(), diagnoseJSON.Summary.UnexpectedErr)

	// Vérifier que les erreurs sont correctement rapportées
	assert.Contains(v.T(), diagnose, "Failed DNS resolution")
}

func (v *datadogURLConnectivitySuite) TestURLConnectivityWithProxy() {
	// Configurer un proxy inexistant pour simuler un échec de connectivité
	params := agentparams.WithAgentConfig(`
site: datadoghq.com
proxy:
  http: http://invalid-proxy:3128
  https: http://invalid-proxy:3128
`)
	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(params)))

	diagnose := getDiagnoseOutput(&v.baseDiagnoseSuite)
	assert.Contains(v.T(), diagnose, "FAIL")
	assert.Contains(v.T(), diagnose, "Failed HTTP connectivity")

	diagnose = getDiagnoseOutput(&v.baseDiagnoseSuite, agentclient.WithArgs([]string{"--json"}))
	diagnoseJSON := unmarshalDiagnose(diagnose)
	require.NotNil(v.T(), diagnoseJSON)
	assert.Greater(v.T(), diagnoseJSON.Summary.Fail, 0)
}

func (v *datadogURLConnectivitySuite) TestURLConnectivityWithInvalidTLS() {
	params := agentparams.WithAgentConfig(`
site: expired.badssl.com
`)
	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(params)))

	diagnose := getDiagnoseOutput(&v.baseDiagnoseSuite)
	assert.Contains(v.T(), diagnose, "FAIL")
	assert.Contains(v.T(), diagnose, "Failed HTTP connectivity")

	diagnose = getDiagnoseOutput(&v.baseDiagnoseSuite, agentclient.WithArgs([]string{"--json"}))
	diagnoseJSON := unmarshalDiagnose(diagnose)
	require.NotNil(v.T(), diagnoseJSON)
	assert.Greater(v.T(), diagnoseJSON.Summary.Fail, 0)
}

func (v *datadogURLConnectivitySuite) TestURLConnectivityWithTimeout() {
	params := agentparams.WithAgentConfig(`
site: 10.255.255.255
`)
	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(params)))

	diagnose := getDiagnoseOutput(&v.baseDiagnoseSuite)
	assert.Contains(v.T(), diagnose, "FAIL")
	assert.Contains(v.T(), diagnose, "Failed HTTP connectivity")

	diagnose = getDiagnoseOutput(&v.baseDiagnoseSuite, agentclient.WithArgs([]string{"--json"}))
	diagnoseJSON := unmarshalDiagnose(diagnose)
	require.NotNil(v.T(), diagnoseJSON)
	assert.Greater(v.T(), diagnoseJSON.Summary.Fail, 0)
}

func (v *datadogURLConnectivitySuite) TestURLConnectivityWithRedirect() {
	params := agentparams.WithAgentConfig(`
site: httpstat.us
`)
	v.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(params)))

	diagnose := getDiagnoseOutput(&v.baseDiagnoseSuite)
	assert.Contains(v.T(), diagnose, "FAIL")
	assert.Contains(v.T(), diagnose, "Failed HTTP connectivity")

	diagnose = getDiagnoseOutput(&v.baseDiagnoseSuite, agentclient.WithArgs([]string{"--json"}))
	diagnoseJSON := unmarshalDiagnose(diagnose)
	require.NotNil(v.T(), diagnoseJSON)
	assert.Greater(v.T(), diagnoseJSON.Summary.Fail, 0)
}
