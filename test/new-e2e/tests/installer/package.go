// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains tests for the datadog installer
package installer

import (
	"fmt"
	"os"
	"regexp"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
)

type packageSuite interface {
	e2e.Suite[environments.Host]

	Name() string
}

type packageBaseSuite struct {
	e2e.BaseSuite[environments.Host]
	host *host.Host

	opts []host.Option
	pkg  string
	arch e2eos.Architecture
	os   e2eos.Descriptor
}

func newPackageSuite(pkg string, os e2eos.Descriptor, arch e2eos.Architecture, opts ...host.Option) packageBaseSuite {
	return packageBaseSuite{
		os:   os,
		arch: arch,
		pkg:  pkg,
		opts: opts,
	}
}

func (s *packageBaseSuite) Name() string {
	return regexp.MustCompile("[^a-zA-Z0-9]+").ReplaceAllString(fmt.Sprintf("%s/%s", s.pkg, s.os), "_")
}

func (s *packageBaseSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.host = host.New(s.T(), s.Env().RemoteHost, s.os, s.arch, s.opts...)
}

func (s *packageBaseSuite) RunInstallScript() {
	env := map[string]string{
		"DD_API_KEY":               "deadbeefdeadbeefdeadbeefdeadbeef",
		"DD_INSTALLER":             "true",
		"DD_NO_AGENT_INSTALL":      "true",
		"TESTING_KEYS_URL":         "keys.datadoghq.com",
		"TESTING_APT_URL":          "apttesting.datad0g.com",
		"TESTING_APT_REPO_VERSION": fmt.Sprintf("pipeline-%s-i7-%s 7", os.Getenv("CI_PIPELINE_ID"), s.os.Architecture),
		"TESTING_YUM_URL":          "yumtesting.datad0g.com",
		"TESTING_YUM_VERSION_PATH": fmt.Sprintf("testing/pipeline-%s-i7/7", os.Getenv("CI_PIPELINE_ID")),
	}
	s.Env().RemoteHost.MustExecute(`bash -c "$(curl -L https://s3.amazonaws.com/dd-agent/scripts/install_script_agent7.sh)"`, components.WithEnvVariables(env))
}

func (s *packageBaseSuite) RemoveInstallerPackage() {
	s.Env().RemoteHost.MustExecute("sudo apt-get remove -y --purge datadog-installer || sudo yum remove -y datadog-installer")
}
