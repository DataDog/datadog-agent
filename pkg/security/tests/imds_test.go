// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/tests/testutils"
)

var _ = declare(TestAWSIMDSv1Request, testOpts{networkIngressEnabled: true})

func TestAWSIMDSv1Request(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_aws_imds_v1_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "aws" && imds.aws.is_imds_v2 == false && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := testutils.IMDSTestServerIP + ":" + strconv.Itoa(testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	t.Run("aws_imds_v1_request", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			response, err := http.Get(fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL))
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_aws_imds_v1_request")
			assert.Equal(t, "request", event.IMDS.Type, "wrong IMDS request type")
			assert.Equal(t, imdsServerAddr, event.IMDS.Host, "wrong IMDS request Host")
			assert.Equal(t, testutils.IMDSSecurityCredentialsURL, event.IMDS.URL, "wrong IMDS request URL")
			assert.Equal(t, "Go-http-client/1.1", event.IMDS.UserAgent, "wrong IMDS request user agent")

			test.validateIMDSSchema(t, event)
		}, "test_rule_aws_imds_v1_request")
	})
}

var _ = declare(TestAWSIMDSv1Response, testOpts{networkIngressEnabled: true})

func TestAWSIMDSv1Response(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		// this rule is added first on purpose to double-check that an IMDSv2 event doesn't trigger and IMDSv1 rule
		{
			ID:         "test_rule_aws_imds_v2_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "aws" && imds.aws.is_imds_v2 == true && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
		{
			ID:         "test_rule_aws_imds_v1_response",
			Expression: fmt.Sprintf(`imds.cloud_provider == "aws" && imds.aws.is_imds_v2 == false && imds.type == "response" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := testutils.IMDSTestServerIP + ":" + strconv.Itoa(testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	t.Run("aws_imds_v1_response", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			response, err := http.Get(fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL))
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_aws_imds_v1_response")
			assert.Equal(t, "response", event.IMDS.Type, "wrong IMDS request Type")
			assert.Equal(t, testutils.AWSIMDSServerTestValue, event.IMDS.Server, "wrong IMDS request Server")
			assert.Equal(t, testutils.AWSSecurityCredentialsTypeTestValue, event.IMDS.AWS.SecurityCredentials.Type, "wrong IMDS request AWS Security Credentials Type")
			assert.Equal(t, testutils.AWSSecurityCredentialsExpirationTestValue, event.IMDS.AWS.SecurityCredentials.ExpirationRaw, "wrong IMDS request AWS Security Credentials ExpirationRaw")
			assert.Equal(t, testutils.AWSSecurityCredentialsAccessKeyIDTestValue, event.IMDS.AWS.SecurityCredentials.AccessKeyID, "wrong IMDS request AWS Security Credentials AccessKeyID")
			assert.Equal(t, testutils.AWSSecurityCredentialsCodeTestValue, event.IMDS.AWS.SecurityCredentials.Code, "wrong IMDS request AWS Security Credentials Code")
			assert.Equal(t, testutils.AWSSecurityCredentialsLastUpdatedTestValue, event.IMDS.AWS.SecurityCredentials.LastUpdated, "wrong IMDS request AWS Security Credentials LastUpdated")

			test.validateIMDSSchema(t, event)
		}, "test_rule_aws_imds_v1_response")
	})
}

var _ = declare(TestAWSIMDSv2Request, testOpts{networkIngressEnabled: true})

func TestAWSIMDSv2Request(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		// this rule is added first on purpose to double-check that an IMDSv2 event doesn't trigger and IMDSv1 rule
		{
			ID:         "test_rule_aws_imds_v1_response",
			Expression: fmt.Sprintf(`imds.cloud_provider == "aws" && imds.aws.is_imds_v2 == false && imds.type == "response" && process.file.name == "%s"`, path.Base(executable)),
		},
		{
			ID:         "test_rule_aws_imds_v2_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "aws" && imds.aws.is_imds_v2 == true && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := testutils.IMDSTestServerIP + ":" + strconv.Itoa(testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	t.Run("aws_imds_v2_request", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL), nil)
			if err != nil {
				return fmt.Errorf("failed to instantiate request: %v", err)
			}
			req.Header.Set("X-aws-ec2-metadata-token", "my_secret_token")
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_aws_imds_v2_request")
			assert.Equal(t, "request", event.IMDS.Type, "wrong IMDS request type")
			assert.Equal(t, imdsServerAddr, event.IMDS.Host, "wrong IMDS request Host")
			assert.Equal(t, testutils.IMDSSecurityCredentialsURL, event.IMDS.URL, "wrong IMDS request URL")
			assert.Equal(t, "Go-http-client/1.1", event.IMDS.UserAgent, "wrong IMDS request user agent")

			test.validateIMDSSchema(t, event)
		}, "test_rule_aws_imds_v2_request")
	})
}

var _ = declare(TestAWSIMDSv2Response, testOpts{networkIngressEnabled: true})

func TestAWSIMDSv2Response(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_aws_imds_v2_response",
			Expression: fmt.Sprintf(`imds.aws.is_imds_v2 == true && imds.type == "response" && imds.aws.security_credentials.type == "%s" && imds.aws.security_credentials.access_key_id == "%s" && process.file.name == "%s"`, testutils.AWSSecurityCredentialsTypeTestValue, testutils.AWSSecurityCredentialsAccessKeyIDTestValue, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := testutils.IMDSTestServerIP + ":" + strconv.Itoa(testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	t.Run("aws_imds_v2_response", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL), nil)
			if err != nil {
				return fmt.Errorf("failed to instantiate request: %v", err)
			}
			req.Header.Set("X-aws-ec2-metadata-token", "my_secret_token")
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			return response.Body.Close()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_aws_imds_v2_response")
			assert.Equal(t, "response", event.IMDS.Type, "wrong IMDS request Type")
			assert.True(t, event.IMDS.AWS.IsIMDSv2, "wrong IMDS version")
			assert.Equal(t, testutils.AWSIMDSServerTestValue, event.IMDS.Server, "wrong IMDS request Server")
			assert.Equal(t, testutils.AWSSecurityCredentialsTypeTestValue, event.IMDS.AWS.SecurityCredentials.Type, "wrong IMDS request AWS Security Credentials Type")
			assert.Equal(t, testutils.AWSSecurityCredentialsExpirationTestValue, event.IMDS.AWS.SecurityCredentials.ExpirationRaw, "wrong IMDS request AWS Security Credentials ExpirationRaw")
			assert.Equal(t, testutils.AWSSecurityCredentialsAccessKeyIDTestValue, event.IMDS.AWS.SecurityCredentials.AccessKeyID, "wrong IMDS request AWS Security Credentials AccessKeyID")
			assert.Equal(t, testutils.AWSSecurityCredentialsCodeTestValue, event.IMDS.AWS.SecurityCredentials.Code, "wrong IMDS request AWS Security Credentials Code")
			assert.Equal(t, testutils.AWSSecurityCredentialsLastUpdatedTestValue, event.IMDS.AWS.SecurityCredentials.LastUpdated, "wrong IMDS request AWS Security Credentials LastUpdated")

			test.validateIMDSSchema(t, event)
		}, "test_rule_aws_imds_v2_response")
	})
}

var _ = declare(TestGCPIMDS, testOpts{networkIngressEnabled: true})

func TestGCPIMDS(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_gcp_imds_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "gcp" && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := testutils.IMDSTestServerIP + ":" + strconv.Itoa(testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	t.Run("gcp_imds_request", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL), nil)
			if err != nil {
				return fmt.Errorf("failed to instantiate request: %v", err)
			}
			req.Header.Set("Metadata-Flavor", "Google")
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_gcp_imds_request")
			assert.Equal(t, "request", event.IMDS.Type, "wrong IMDS request type")
			assert.Equal(t, imdsServerAddr, event.IMDS.Host, "wrong IMDS request Host")
			assert.Equal(t, testutils.IMDSSecurityCredentialsURL, event.IMDS.URL, "wrong IMDS request URL")
			assert.Equal(t, "Go-http-client/1.1", event.IMDS.UserAgent, "wrong IMDS request user agent")

			test.validateIMDSSchema(t, event)
		}, "test_rule_gcp_imds_request")
	})
}

var _ = declare(TestAzureIMDS, testOpts{networkIngressEnabled: true})

func TestAzureIMDS(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_azure_imds_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "azure" && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := testutils.IMDSTestServerIP + ":" + strconv.Itoa(testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	t.Run("azure_imds_request", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL), nil)
			if err != nil {
				return fmt.Errorf("failed to instantiate request: %v", err)
			}
			req.Header.Set("Metadata", "true")
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_azure_imds_request")
			assert.Equal(t, "request", event.IMDS.Type, "wrong IMDS request type")
			assert.Equal(t, imdsServerAddr, event.IMDS.Host, "wrong IMDS request Host")
			assert.Equal(t, testutils.IMDSSecurityCredentialsURL, event.IMDS.URL, "wrong IMDS request URL")
			assert.Equal(t, "Go-http-client/1.1", event.IMDS.UserAgent, "wrong IMDS request user agent")

			test.validateIMDSSchema(t, event)
		}, "test_rule_azure_imds_request")
	})
}

var _ = declare(TestIBMIMDS, testOpts{networkIngressEnabled: true})

func TestIBMIMDS(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_idbm_imds_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "ibm" && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := testutils.IMDSTestServerIP + ":" + strconv.Itoa(testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	t.Run("ibm_imds_request", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL), nil)
			if err != nil {
				return fmt.Errorf("failed to instantiate request: %v", err)
			}
			req.Header.Set("Metadata-Flavor", "ibm")
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_idbm_imds_request")
			assert.Equal(t, "request", event.IMDS.Type, "wrong IMDS request type")
			assert.Equal(t, imdsServerAddr, event.IMDS.Host, "wrong IMDS request Host")
			assert.Equal(t, testutils.IMDSSecurityCredentialsURL, event.IMDS.URL, "wrong IMDS request URL")
			assert.Equal(t, "Go-http-client/1.1", event.IMDS.UserAgent, "wrong IMDS request user agent")

			test.validateIMDSSchema(t, event)
		}, "test_rule_idbm_imds_request")
	})
}

var _ = declare(TestOracleIMDS, testOpts{networkIngressEnabled: true})

func TestOracleIMDS(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_oracle_imds_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "oracle" && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := testutils.IMDSTestServerIP + ":" + strconv.Itoa(testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	t.Run("oracle_imds_request", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL), nil)
			if err != nil {
				return fmt.Errorf("failed to instantiate request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer Oracle")
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_oracle_imds_request")
			assert.Equal(t, "request", event.IMDS.Type, "wrong IMDS request type")
			assert.Equal(t, imdsServerAddr, event.IMDS.Host, "wrong IMDS request Host")
			assert.Equal(t, testutils.IMDSSecurityCredentialsURL, event.IMDS.URL, "wrong IMDS request URL")
			assert.Equal(t, "Go-http-client/1.1", event.IMDS.UserAgent, "wrong IMDS request user agent")

			test.validateIMDSSchema(t, event)
		}, "test_rule_oracle_imds_request")
	})
}

var _ = declare(TestIMDSProcessContext, testOpts{networkIngressEnabled: true})

func TestIMDSProcessContext(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_oracle_imds_request",
			Expression: fmt.Sprintf(`imds.cloud_provider == "oracle" && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
		{
			ID:         "test_imds_process_context",
			Expression: `open.file.path == "{{.Root}}/test-open" && open.flags & O_CREAT != 0`,
		},
		// check dumps
	}

	// create dummy interface
	dummy, err := testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake IMDS server
	imdsServerAddr := testutils.IMDSTestServerIP + ":" + strconv.Itoa(testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err = testutils.StopIMDSserver(imdsServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	testFile, testFilePtr, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("imds_process_context", ifSyscallSupported("SYS_OPEN", func(t *testing.T, syscallNB uintptr) {
		defer os.Remove(testFile)

		test.WaitSignalFromRule(t, func() error {
			// make request first to populate process cache
			response, err := http.Get(fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL))
			if err != nil {
				return fmt.Errorf("failed to query IMDS server: %v", err)
			}
			defer response.Body.Close()

			fd, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), syscall.O_CREAT, 0755)
			if errno != 0 {
				return error(errno)
			}
			return syscall.Close(int(fd))
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_imds_process_context")

			// check if the process has the correct IMDS credentials context
			assert.NotNil(t, event.ProcessCacheEntry.Process.AWSSecurityCredentials, "empty IMDS context")
			if len(event.ProcessCacheEntry.Process.AWSSecurityCredentials) > 0 {
				creds := event.ProcessCacheEntry.Process.AWSSecurityCredentials[0]
				assert.Equal(t, testutils.AWSSecurityCredentialsTypeTestValue, creds.Type, "wrong IMDS context AWS Security Credentials Type")
				assert.Equal(t, testutils.AWSSecurityCredentialsExpirationTestValue, creds.ExpirationRaw, "wrong IMDS context AWS Security Credentials ExpirationRaw")
				assert.Equal(t, testutils.AWSSecurityCredentialsAccessKeyIDTestValue, creds.AccessKeyID, "wrong IMDS context AWS Security Credentials AccessKeyID")
				assert.Equal(t, testutils.AWSSecurityCredentialsCodeTestValue, creds.Code, "wrong IMDS context AWS Security Credentials Code")
				assert.Equal(t, testutils.AWSSecurityCredentialsLastUpdatedTestValue, creds.LastUpdated, "wrong IMDS context AWS Security Credentials LastUpdated")
			}

			test.validateOpenSchema(t, event)
		}, "test_imds_process_context")
	}))
}

var _ = declare(TestIMDSCredentialAttribution, testOpts{networkIngressEnabled: true})

// TestIMDSCredentialAttribution keeps the broker and both clients in distinct
// processes. A same-process HTTP fixture cannot detect broker self-attribution.
func TestIMDSCredentialAttribution(t *testing.T) {
	SkipIfNotAvailable(t)
	checkNetworkCompatibility(t)
	if testEnvironment == DockerEnvironment {
		t.Skip("socket ownership uses host pids, unlike child process pids in Docker")
	}
	if !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load veth: %s, %v", out, err)
		}
	}

	keySeed := uint64(time.Now().UnixNano())
	keys := []string{
		fmt.Sprintf("ASIA%016X", keySeed),
		fmt.Sprintf("ASIA%016X", keySeed+1),
	}
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_credential_attribution_response",
			Expression: fmt.Sprintf(`imds.type == "response" && imds.aws.security_credentials.access_key_id in ["%s", "%s"]`, keys[0], keys[1]),
		},
		{
			ID:         "test_credential_attribution_open",
			Expression: `open.file.path in ["{{.Root}}/credential-client-0", "{{.Root}}/credential-client-1", "{{.Root}}/credential-broker"] && open.flags & O_CREAT != 0`,
		},
	}
	dummy, err := testutils.CreateDummyInterface(testutils.CSMPodIdentityDummyInterface, testutils.EKSPodIdentityTestServerCIDR)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, testutils.RemoveDummyInterface(dummy)) })
	test, err := newTestModule(t, nil, ruleDefs)
	require.NoError(t, err)
	t.Cleanup(test.CloseTest)

	// Create sockets after starting the probe so their owners are recorded.
	listener, err := net.Listen("tcp", net.JoinHostPort(testutils.EKSPodIdentityTestServerIP, "0"))
	require.NoError(t, err)
	server := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			index, err := strconv.Atoi(r.URL.Query().Get("client"))
			if err != nil || index < 0 || index >= len(keys) {
				http.Error(w, "unknown client", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]string{
				"AccessKeyId": keys[index],
				"Expiration":  testutils.AWSSecurityCredentialsExpirationTestValue,
			}); err != nil {
				t.Errorf("failed to write credentials: %v", err)
			}
		}),
	}
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	t.Cleanup(func() {
		assert.NoError(t, testutils.StopIMDSserver(server))
		assert.ErrorIs(t, <-serverDone, http.ErrServerClosed)
	})
	syscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	require.NoError(t, err)
	brokerPID := uint32(os.Getpid())
	type clientProcess struct {
		pid  uint32
		open func() error
	}
	var clients []clientProcess
	for i, key := range keys {
		marker, _, err := test.Path(fmt.Sprintf("credential-client-%d", i))
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		t.Cleanup(cancel)
		cmd := exec.CommandContext(ctx, syscallTester,
			"-credentials-url", fmt.Sprintf("http://%s%s?client=%d", listener.Addr(), testutils.EKSPodIdentityCredentialsURL, i),
			"-credentials-open-path", marker)
		cmd.Stderr = os.Stderr
		stdin, err := cmd.StdinPipe()
		require.NoError(t, err)
		stdout, err := cmd.StdoutPipe()
		require.NoError(t, err)
		scanner := bufio.NewScanner(stdout)
		readLine := func(want string) error {
			if !scanner.Scan() || scanner.Text() != want {
				return fmt.Errorf("expected helper output %q, got %q (scan error: %v)", want, scanner.Text(), scanner.Err())
			}
			return nil
		}
		var clientPID uint32
		var sent, received bool
		test.WaitSignals(t, func() error {
			if err := cmd.Start(); err != nil {
				cancel()
				return err
			}
			t.Cleanup(func() {
				_ = stdin.Close()
				cancel()
				_ = cmd.Wait()
			})
			clientPID = uint32(cmd.Process.Pid)
			return readLine("ready")
		}, func(event *model.Event, rule *rules.Rule) error {
			if rule.ID != "test_credential_attribution_response" || event.IMDS.AWS.SecurityCredentials.AccessKeyID != key {
				return errSkipEvent
			}
			assert.Equal(t, uint32(model.CredentialSourceEKSPodIdentity), event.IMDS.CredentialSource)
			switch model.NetworkDirection(event.NetworkContext.NetworkDirection) {
			case model.Egress:
				assert.Equal(t, brokerPID, event.PIDContext.Pid)
				sent = true
			case model.Ingress:
				assert.Equal(t, clientPID, event.PIDContext.Pid)
				received = true
			}
			test.validateIMDSSchema(t, event)
			if sent && received {
				return nil
			}
			return errSkipEvent
		})
		clients = append(clients, clientProcess{
			pid: clientPID,
			open: func() error {
				if _, err := fmt.Fprintln(stdin, "open"); err != nil {
					return err
				}
				return readLine("opened")
			},
		})
	}

	// Inspect subsequent events only after both responses have been dispatched.
	// Waiting for cache mutation inside a response callback would deadlock it.
	for i, client := range clients {
		test.WaitSignalFromRule(t, client.open, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, client.pid, event.PIDContext.Pid)
			var receivedKeys []string
			for _, credentials := range event.ProcessCacheEntry.AWSSecurityCredentials {
				receivedKeys = append(receivedKeys, credentials.AccessKeyID)
			}
			assert.ElementsMatch(t, []string{keys[i]}, receivedKeys)
			test.validateOpenSchema(t, event)
		}, "test_credential_attribution_open")
	}

	marker, _, err := test.Path("credential-broker")
	require.NoError(t, err)
	test.WaitSignalFromRule(t, func() error {
		file, err := os.OpenFile(marker, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		return file.Close()
	}, func(event *model.Event, _ *rules.Rule) {
		assert.Equal(t, brokerPID, event.PIDContext.Pid)
		for _, credentials := range event.ProcessCacheEntry.AWSSecurityCredentials {
			assert.NotContains(t, keys, credentials.AccessKeyID, "broker must not accumulate served keys")
		}
		test.validateOpenSchema(t, event)
	}, "test_credential_attribution_open")
}

var _ = declare(TestEKSPodIdentityResponse, testOpts{networkIngressEnabled: true})

func TestEKSPodIdentityResponse(t *testing.T) {
	SkipIfNotAvailable(t)

	checkNetworkCompatibility(t)

	if testEnvironment != DockerEnvironment && !env.IsContainerized() {
		if out, err := loadModule("veth"); err != nil {
			t.Fatalf("couldn't load 'veth' module: %s,%v", string(out), err)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_eks_pod_identity_request",
			Expression: fmt.Sprintf(`imds.credential_source == EKS_POD_IDENTITY && imds.type == "request" && process.file.name == "%s"`, path.Base(executable)),
		},
		{
			ID:         "test_rule_eks_pod_identity_response",
			Expression: fmt.Sprintf(`imds.credential_source == EKS_POD_IDENTITY && imds.type == "response" && imds.aws.security_credentials.access_key_id == "%s" && process.file.name == "%s"`, testutils.AWSSecurityCredentialsAccessKeyIDTestValue, path.Base(executable)),
		},
	}

	// create a dummy interface holding the EKS Pod Identity Agent address
	dummy, err := testutils.CreateDummyInterface(testutils.CSMPodIdentityDummyInterface, testutils.EKSPodIdentityTestServerCIDR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = testutils.RemoveDummyInterface(dummy); err != nil {
			t.Fatal(err)
		}
	}()

	// create fake EKS Pod Identity Agent
	podIdentityAddr := testutils.EKSPodIdentityTestServerIP + ":" + strconv.Itoa(testutils.EKSPodIdentityTestServerPort)
	podIdentityServer := testutils.CreateEKSPodIdentityServer(podIdentityAddr)
	defer func() {
		if err = testutils.StopIMDSserver(podIdentityServer); err != nil {
			t.Fatal(err)
		}
	}()

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	queryPodIdentity := func() error {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", podIdentityAddr, testutils.EKSPodIdentityCredentialsURL), nil)
		if err != nil {
			return fmt.Errorf("failed to instantiate request: %v", err)
		}
		// the AWS SDK authenticates to the agent with the pod service account token
		req.Header.Set("Authorization", "Bearer my_service_account_token")

		response, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to query EKS Pod Identity Agent: %v", err)
		}
		defer response.Body.Close()

		return nil
	}

	t.Run("eks_pod_identity_request", func(t *testing.T) {
		test.WaitSignalFromRule(t, queryPodIdentity, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_eks_pod_identity_request")
			assert.Equal(t, "request", event.IMDS.Type, "wrong event type")
			assert.Equal(t, uint32(model.CredentialSourceEKSPodIdentity), event.IMDS.CredentialSource, "wrong credential source")
			assert.Equal(t, podIdentityAddr, event.IMDS.Host, "wrong Host")
			assert.Equal(t, testutils.EKSPodIdentityCredentialsURL, event.IMDS.URL, "wrong URL")
			// the Pod Identity Agent has no v1/v2 notion
			assert.False(t, event.IMDS.AWS.IsIMDSv2, "is_imds_v2 should not be set for EKS Pod Identity")

			test.validateIMDSSchema(t, event)
		}, "test_rule_eks_pod_identity_request")
	})

	t.Run("eks_pod_identity_response", func(t *testing.T) {
		test.WaitSignalFromRule(t, queryPodIdentity, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_eks_pod_identity_response")
			assert.Equal(t, "response", event.IMDS.Type, "wrong event type")
			assert.Equal(t, uint32(model.CredentialSourceEKSPodIdentity), event.IMDS.CredentialSource, "wrong credential source")
			// the endpoint sends no identifying header, so it is resolved as AWS
			assert.Equal(t, model.IMDSAWSCloudProvider, event.IMDS.CloudProvider, "wrong cloud provider")
			assert.Equal(t, testutils.AWSSecurityCredentialsAccessKeyIDTestValue, event.IMDS.AWS.SecurityCredentials.AccessKeyID, "wrong AccessKeyID")
			assert.Equal(t, testutils.AWSSecurityCredentialsExpirationTestValue, event.IMDS.AWS.SecurityCredentials.ExpirationRaw, "wrong ExpirationRaw")
			// fields that only IMDS sends are absent from a Pod Identity response
			assert.Empty(t, event.IMDS.AWS.SecurityCredentials.Code, "Code should be empty")
			assert.Empty(t, event.IMDS.AWS.SecurityCredentials.Type, "Type should be empty")
			assert.Empty(t, event.IMDS.AWS.SecurityCredentials.LastUpdated, "LastUpdated should be empty")
			assert.False(t, event.IMDS.AWS.IsIMDSv2, "is_imds_v2 should not be set for EKS Pod Identity")

			test.validateIMDSSchema(t, event)
		}, "test_rule_eks_pod_identity_response")
	})
}
