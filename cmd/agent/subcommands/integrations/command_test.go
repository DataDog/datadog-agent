// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package integrations

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestInstallCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"integration", "install", "foo==1.0", "-v"},
		install,
		func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, []string{"foo==1.0"}, cliParams.args)
			require.Equal(t, 1, cliParams.verbose)
			require.Equal(t, false, secretParams.Enabled)
			require.Equal(t, true, coreParams.ConfigMissingOK())
		})
}

func TestInstallSkipVerificationCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"integration", "install", "foo==1.0", "--unsafe-disable-verification"},
		install,
		func(cliParams *cliParams, coreParams core.BundleParams) {
			require.Equal(t, []string{"foo==1.0"}, cliParams.args)
			require.Equal(t, true, cliParams.unsafeDisableVerification)
		})
}

func TestRemoveCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"integration", "remove", "foo"},
		remove,
		func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, []string{"foo"}, cliParams.args)
			require.Equal(t, 0, cliParams.verbose)
			require.Equal(t, false, secretParams.Enabled)
			require.Equal(t, true, coreParams.ConfigMissingOK())
		})
}

func TestFreezeCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"integration", "freeze"},
		list,
		func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, []string{}, cliParams.args)
			require.Equal(t, 0, cliParams.verbose)
			require.Equal(t, false, secretParams.Enabled)
			require.Equal(t, true, coreParams.ConfigMissingOK())
		})
}

func TestShowCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"integration", "show", "foo"},
		show,
		func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, []string{"foo"}, cliParams.args)
			require.Equal(t, 0, cliParams.verbose)
			require.Equal(t, false, secretParams.Enabled)
			require.Equal(t, true, coreParams.ConfigMissingOK())
		})
}

func TestRegexMatch(t *testing.T) {
	testCases := []struct {
		regex      *regexp.Regexp
		text       string
		matches    bool
		firstMatch string
	}{
		{yamlFileNameRe, "abc_def.yaml", true, "abc_def.yaml"},
		{yamlFileNameRe, "abcdyaml", false, ""},
		{yamlFileNameRe, ".yaml", false, ""},
		{yamlFileNameRe, "ab\\.yaml", false, ""},
		{yamlFileNameRe, "abc.yaml.disabled", true, "abc.yaml.disabled"},

		{wheelPackageNameRe, "Name: ", false, ""},
		{wheelPackageNameRe, "Name: datadog postgres", true, "Name: datadog"},
		{wheelPackageNameRe, "Name:  datadog-postgres", false, ""},
		{wheelPackageNameRe, "Name: datadog-postgres", true, "Name: datadog-postgres"},
		{wheelPackageNameRe, "xx Name: datadog-postgres", true, "Name: datadog-postgres"},

		{pep440VersionStringRe, "1.3.4b1", true, "1.3.4b1"},
		{pep440VersionStringRe, "1.2.3", true, "1.2.3"},
		{pep440VersionStringRe, "1.2.3aaa", true, "1.2.3aaa"},
		{pep440VersionStringRe, "1.2.3a4b", false, ""},
		{pep440VersionStringRe, "1.2", false, ""},
		{pep440VersionStringRe, "a1.2.3", false, ""},
		{pep440VersionStringRe, "1.2.3\\d", false, ""},
	}

	for _, test := range testCases {
		if assert.Equal(t, test.matches, test.regex.MatchString(test.text), test.text) {
			assert.Equal(t, test.firstMatch, test.regex.FindString(test.text))
		}
	}

	pep440MatchNamesTest := "1.3.4b1"
	matches := pep440VersionStringRe.FindStringSubmatch(pep440MatchNamesTest)
	require.NotNil(t, matches)
	require.Len(t, matches, 4)
	require.Equal(t, pep440MatchNamesTest, matches[0])

	for subexpName, expected := range map[string]string{
		"release":          "1.3.4",
		"preReleaseType":   "b",
		"preReleaseNumber": "1",
	} {
		subexpIndex := pep440VersionStringRe.SubexpIndex(subexpName)
		require.NotEqual(t, -1, subexpIndex)
		require.Equal(t, expected, matches[subexpIndex])
	}
}
