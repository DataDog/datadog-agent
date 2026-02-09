// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package jmxfetch

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	ec2docker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/jmxfetch"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/secretsmanager"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

//go:embed testdata/docker-labels.yaml
var jmxFetchADLabels string

const testCertsARN = "arn:aws:secretsmanager:us-east-1:376334461865:secret:agent-e2e-jmxfetch-test-certs-20250107-d3WPao"

type jmxfetchNixTest struct {
	e2e.BaseSuite[environments.DockerHost]

	fips bool
}

func testJMXFetchNix(t *testing.T, mtls bool, fips bool) {
	adLabelsManifest, err := makeADLabelsManifest(mtls, fips)
	require.NoError(t, err)

	extraManifests := []docker.ComposeInlineManifest{
		jmxfetch.DockerComposeManifest,
		*adLabelsManifest,
	}

	if mtls {
		mtlsManifest, err := makeMtlsManifest(fips)
		require.NoError(t, err)
		extraManifests = append(extraManifests, *mtlsManifest)
	}

	t.Parallel()

	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(
		awsdocker.Provisioner(
			awsdocker.WithRunOptions(
				ec2docker.WithAgentOptions(
					dockeragentparams.WithLogs(),
					dockeragentparams.WithJMX(),
					choice(fips, dockeragentparams.WithFIPS(), none),
					dockeragentparams.WithExtraComposeInlineManifest(extraManifests...),
				),
				choice(mtls, ec2docker.WithPreAgentInstallHook(fetchCertificates), none),
			),
		)),
		e2e.WithStackName(fmt.Sprintf("jmxfetchnixtest-fips_%v-mtls_%v", fips, mtls)),
	}

	e2e.Run(t,
		&jmxfetchNixTest{fips: fips},
		suiteParams...,
	)
}

func TestJMXFetchNix(t *testing.T) {
	testJMXFetchNix(t, false, false)
}

func TestJMXFetchNixFIPS(t *testing.T) {
	testJMXFetchNix(t, false, true)
}

func TestJMXFetchNixMtls(t *testing.T) {
	testJMXFetchNix(t, true, false)
}

func TestJMXFetchNixMtlsFIPS(t *testing.T) {
	testJMXFetchNix(t, true, true)
}

func (j *jmxfetchNixTest) Test_FakeIntakeReceivesJMXFetchMetrics() {
	metricNames := []string{
		"test.e2e.jmxfetch.counter_100",
		"test.e2e.jmxfetch.gauge_200",
		"test.e2e.jmxfetch.increment_counter",
	}
	start := time.Now()
	j.EventuallyWithT(func(c *assert.CollectT) {
		for _, metricName := range metricNames {
			metrics, err := j.Env().FakeIntake.Client().
				FilterMetrics(metricName, client.WithMetricValueHigherThan(0))
			assert.NoError(c, err)
			assert.NotEmpty(c, metrics, "no metrics found for", metricName)
		}
	}, 5*time.Minute, 10*time.Second)
	j.T().Logf("Started: %v and took %v", start, time.Since(start))

	// Helpful debug when things fail
	if j.T().Failed() {
		names, err := j.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(j.T(), err)
		for _, name := range names {
			j.T().Logf("Got metric: %q", name)
		}
		for _, metricName := range metricNames {
			tjc, err := j.Env().FakeIntake.Client().FilterMetrics(metricName)
			assert.NoError(j.T(), err)
			assert.NotEmpty(j.T(), tjc, "Filter metrics was empty for %q", metricName)
			if len(tjc) > 0 {
				for _, point := range tjc[0].Points {
					j.T().Logf("Found metrics: %q \n%v - %v \n%q", tjc[0].Metric, point, point.Value, tjc[0].Type)
				}
			}
		}
	}
}

func (j *jmxfetchNixTest) TestJMXListCollectedWithRateMetrics() {
	status, err := j.Env().Agent.Client.JMX(agentclient.WithArgs([]string{"list", "collected", "with-rate-metrics"}))
	require.NoError(j.T(), err)
	assert.NotEmpty(j.T(), status.Content)

	lines := strings.Split(status.Content, "\n")
	var consoleReporterOut []string
	var foundShouldBe100, foundShouldBe200, foundIncrementCounter bool
	for _, line := range lines {
		if strings.Contains(line, "ConsoleReporter") {
			consoleReporterOut = append(consoleReporterOut, line)
			if strings.Contains(line, "dd.test.sample:name=default,type=simple") {
				if strings.Contains(line, "ShouldBe100") {
					foundShouldBe100 = true
				}
				if strings.Contains(line, "ShouldBe200") {
					foundShouldBe200 = true
				}
				if strings.Contains(line, "IncrementCounter") {
					foundIncrementCounter = true
				}
			}
		}
	}

	assert.NotEmpty(j.T(), consoleReporterOut, "Did not find ConsoleReporter output in status")
	assert.True(j.T(), foundShouldBe100,
		"Did not find bean name: dd.test.sample:name=default,type=simple - Attribute name: ShouldBe100  - Attribute type: java.lang.Integer")
	assert.True(j.T(), foundShouldBe200,
		"Did not find bean name: dd.test.sample:name=default,type=simple - Attribute name: ShouldBe200  - Attribute type: java.lang.Double")
	assert.True(j.T(), foundIncrementCounter,
		"Did not find bean name: dd.test.sample:name=default,type=simple - Attribute name: IncrementCounter  - Attribute type: java.lang.Integer")

	// Helpful debug when things fail
	if j.T().Failed() {
		for _, line := range consoleReporterOut {
			j.T().Log(line)
		}
	}
}

func (j *jmxfetchNixTest) TestJMXFIPSMode() {
	env, err := j.Env().Docker.Client.ExecuteCommandWithErr(j.Env().Agent.ContainerName, "env")
	require.NoError(j.T(), err)
	if j.fips {
		assert.Contains(j.T(), env, "JAVA_TOOL_OPTIONS=--module-path")
	} else {
		assert.Contains(j.T(), env, "JAVA_TOOL_OPTIONS=\n")
	}
}

func none[T any](_ T) error { return nil }

func choice[T any](cond bool, then, otherwise T) T {
	if cond {
		return then
	}
	return otherwise
}

func fetchCertificates(awsEnv *aws.Environment, h *remote.Host) (pulumi.Resource, error) {
	args := secretsmanager.LookupSecretVersionOutputArgs{SecretId: pulumi.String(testCertsARN)}
	certs := secretsmanager.LookupSecretVersionOutput(awsEnv.Ctx(), args, awsEnv.WithProvider(config.ProviderAWS))

	// Commands here are queued and executed later.
	mkdirCmd, path, err := h.OS.FileManager().HomeDirectory("certs")
	if err != nil {
		return nil, fmt.Errorf("failed to create mkdir command: %w", err)
	}
	runner := h.OS.Runner()

	unpack, err := runner.Command("certs", &command.Args{
		Create: pulumi.String(fmt.Sprintf("base64 -d | tar -C%q -xzf-", path)),
		Stdin:  certs.SecretBinary(),
	}, utils.PulumiDependsOn(mkdirCmd))

	if err != nil {
		return nil, fmt.Errorf("failed to create decode command: %w", err)
	}

	return unpack, nil
}

type serviceDesc struct {
	Secrets     []string
	Labels      map[string]string
	Environment map[string]string
}

type secretDesc struct {
	File string
}

type composeDesc struct {
	Services map[string]serviceDesc
	Secrets  map[string]secretDesc
}

// makeMtlsManifest provides secrets and configures jmx test app to use ssl.
func makeMtlsManifest(fips bool) (*docker.ComposeInlineManifest, error) {
	appSecrets := []string{"pkcs12-java-app-keystore", "pkcs12-java-app-truststore"}
	jmxfetchSecrets := []string{"pkcs12-jmxfetch-keystore", "pkcs12-jmxfetch-truststore"}
	if fips {
		jmxfetchSecrets = []string{"bcfks-jmxfetch-keystore", "bcfks-jmxfetch-truststore"}
	}

	appFlags := []string{
		"-Dcom.sun.management.jmxremote.ssl.need.client.auth=true",
		"-Djavax.net.ssl.keyStore=/run/secrets/pkcs12-java-app-keystore",
		"-Djavax.net.ssl.keyStorePassword=changeit",
		"-Djavax.net.ssl.trustStore=/run/secrets/pkcs12-java-app-truststore",
		"-Djavax.net.ssl.trustStorePassword=changeit",
	}

	desc := composeDesc{
		Services: map[string]serviceDesc{
			"jmx-test-app": {
				Secrets: appSecrets,
				Environment: map[string]string{
					"SSL_MODE":          "true",
					"JAVA_TOOL_OPTIONS": strings.Join(appFlags, " "),
					"DUMMY_UPDATE_2":    "remove_before_merging",
				},
			},
			"agent": {
				Secrets: jmxfetchSecrets,
			},
		},

		Secrets: map[string]secretDesc{
			"bcfks-java-app-keystore":    {"../certs/bcfks/java-app-keystore"},
			"bcfks-java-app-truststore":  {"../certs/bcfks/java-app-truststore"},
			"bcfks-jmxfetch-keystore":    {"../certs/bcfks/jmxfetch-keystore"},
			"bcfks-jmxfetch-truststore":  {"../certs/bcfks/jmxfetch-truststore"},
			"pkcs12-java-app-keystore":   {"../certs/pkcs12/java-app-keystore"},
			"pkcs12-java-app-truststore": {"../certs/pkcs12/java-app-truststore"},
			"pkcs12-jmxfetch-keystore":   {"../certs/pkcs12/jmxfetch-keystore"},
			"pkcs12-jmxfetch-truststore": {"../certs/pkcs12/jmxfetch-truststore"},
		},
	}

	out, err := yaml.Marshal(&desc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal compose manifest yaml: %w", err)
	}

	return &docker.ComposeInlineManifest{
		Name:    "jmx-test-tls-manifest",
		Content: pulumi.String(string(out)),
	}, nil
}

type checksLabel map[string]checkConfig

type checkConfig struct {
	InitConfig json.RawMessage `json:"init_config"`
	Instances  []checkInstance
}

type checkInstance struct {
	Host               string  `json:"host"`
	Port               string  `json:"port"`
	RmiRegistrySsl     *bool   `json:"rmi_registry_ssl,omitempty"`
	KeyStorePath       *string `json:"key_store_path,omitempty"`
	KeyStorePassword   *string `json:"key_store_password,omitempty"`
	TrustStorePath     *string `json:"trust_store_path,omitempty"`
	TrustStorePassword *string `json:"trust_store_password,omitempty"`
	JavaOptions        *string `json:"java_options"`
}

var defaultJavaPassword = "changeit"
var javaOptionsNoCertCheck = "-Djdk.rmi.ssl.client.enableEndpointIdentification=false"

const adLabelName = "com.datadoghq.ad.checks"

func makeADLabelsManifest(mtls bool, fips bool) (*docker.ComposeInlineManifest, error) {
	manifestYaml := jmxFetchADLabels

	if mtls {
		var err error
		manifestYaml, err = withInstance(manifestYaml, func(instance *checkInstance) {
			secretPrefix := "/run/secrets"
			keystore, truststore := "pkcs12-jmxfetch-keystore", "pkcs12-jmxfetch-truststore"
			if fips {
				keystore, truststore = "bcfks-jmxfetch-keystore", "bcfks-jmxfetch-truststore"
			}
			keystorePath := path.Join(secretPrefix, keystore)
			truststorePath := path.Join(secretPrefix, truststore)

			instance.RmiRegistrySsl = &mtls
			instance.KeyStorePath = &keystorePath
			instance.KeyStorePassword = &defaultJavaPassword
			instance.TrustStorePath = &truststorePath
			instance.TrustStorePassword = &defaultJavaPassword
			instance.JavaOptions = &javaOptionsNoCertCheck
		})
		if err != nil {
			return nil, err
		}
	}

	return &docker.ComposeInlineManifest{
		Name:    "jmx-test-app-labels",
		Content: pulumi.String(manifestYaml),
	}, nil
}

// withInstance digs out check instance out of the testdata config and
// puts it back after callback has modified it.
func withInstance(manifestYaml string, cb func(*checkInstance)) (string, error) {
	var manifest composeDesc
	err := yaml.Unmarshal([]byte(manifestYaml), &manifest)
	if err != nil {
		return "", fmt.Errorf("failed to parse label manifest: %w", err)
	}

	labels := manifest.Services["jmx-test-app"].Labels
	labelJSON := []byte(labels[adLabelName])

	var label checksLabel
	err = json.Unmarshal(labelJSON, &label)
	if err != nil {
		return "", fmt.Errorf("failed to parse label %q: %w", adLabelName, err)
	}

	cb(&label["test"].Instances[0])

	labelJSON, err = json.Marshal(label)
	if err != nil {
		return "", fmt.Errorf("failed to marshal label json: %w", err)
	}

	labels[adLabelName] = string(labelJSON)
	manifestBytes, err := yaml.Marshal(&manifest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal manifest yaml: %w", err)
	}

	return string(manifestBytes), nil
}
