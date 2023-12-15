// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
)

// fromConfig

func TestFromConfig(t *testing.T) {
	config.Mock(t)
	config.Datadog.SetWithoutSource("hostname", "test-hostname")

	hostname, err := fromConfig(context.TODO(), "")
	require.NoError(t, err)
	assert.Equal(t, "test-hostname", hostname)
}

func TestFromConfigInvalid(t *testing.T) {
	config.Mock(t)
	config.Datadog.SetWithoutSource("hostname", "hostname_with_underscore")

	_, err := fromConfig(context.TODO(), "")
	assert.Error(t, err)
}

// fromHostnameFile

func setupHostnameFile(t *testing.T, content string) {
	dir := t.TempDir()
	destFile, err := os.CreateTemp(dir, "test-hostname-file-config-")
	require.NoError(t, err, "Could not create tmp file: %s", err)

	err = os.WriteFile(destFile.Name(), []byte(content), os.ModePerm)
	require.NoError(t, err, "Could not write to tmp file %s: %s", destFile.Name(), err)

	config.Mock(t)
	config.Datadog.SetWithoutSource("hostname_file", destFile.Name())

	destFile.Close()
}

func TestFromHostnameFile(t *testing.T) {
	setupHostnameFile(t, "expectedfilehostname")

	hostname, err := fromHostnameFile(context.TODO(), "")
	require.NoError(t, err)
	assert.Equal(t, "expectedfilehostname", hostname)
}

func TestFromHostnameFileWhitespaceTrim(t *testing.T) {
	setupHostnameFile(t, "  \n\r expectedfilehostname  \r\n\n ")

	hostname, err := fromHostnameFile(context.TODO(), "")
	require.NoError(t, err)
	assert.Equal(t, "expectedfilehostname", hostname)
}

func TestFromHostnameFileNoFileName(t *testing.T) {
	config.Mock(t)
	config.Datadog.SetWithoutSource("hostname_file", "")

	_, err := fromHostnameFile(context.TODO(), "")
	assert.NotNil(t, err)
}

func TestFromHostnameFileInvalid(t *testing.T) {
	setupHostnameFile(t, "invalid_hostname_with_underscore")

	_, err := fromHostnameFile(context.TODO(), "")
	assert.Error(t, err)
}

// fromFargate

func TestFromFargate(t *testing.T) {
	defer func() { isFargateInstance = fargate.IsFargateInstance }()

	isFargateInstance = func() bool { return true }
	hostname, err := fromFargate(context.TODO(), "")
	require.NoError(t, err)
	assert.Equal(t, "", hostname)

	isFargateInstance = func() bool { return false }
	_, err = fromFargate(context.TODO(), "")
	assert.Error(t, err)
}

// fromFQDN

func TestFromFQDN(t *testing.T) {
	defer func() {
		// making isOSHostnameUsable return true
		osHostnameUsable = isOSHostnameUsable
		fqdnHostname = getSystemFQDN
	}()
	osHostnameUsable = func(ctx context.Context) bool { return true }
	fqdnHostname = func() (string, error) { return "fqdn-hostname", nil }

	config.Mock(t)
	config.Datadog.SetWithoutSource("hostname_fqdn", false)

	_, err := fromFQDN(context.TODO(), "")
	assert.Error(t, err)

	config.Datadog.SetWithoutSource("hostname_fqdn", true)

	hostname, err := fromFQDN(context.TODO(), "")
	assert.NoError(t, err)
	assert.Equal(t, "fqdn-hostname", hostname)
}

// fromOS

func TestFromOS(t *testing.T) {
	defer func() {
		// making isOSHostnameUsable return true
		osHostnameUsable = isOSHostnameUsable
	}()
	osHostnameUsable = func(ctx context.Context) bool { return true }
	expected, _ := os.Hostname()

	hostname, err := fromOS(context.TODO(), "")
	assert.NoError(t, err)
	assert.Equal(t, expected, hostname)

	_, err = fromOS(context.TODO(), "previous-hostname")
	assert.Error(t, err)
}

// fromEC2

func TestFromEc2DefaultHostname(t *testing.T) {
	// This test that when a default EC2 hostname is detected we fallback on EC2 instance ID
	defer func() { ec2GetInstanceID = ec2.GetInstanceID }()

	// make AWS provider return an error
	ec2GetInstanceID = func(context.Context) (string, error) { return "", fmt.Errorf("some error") }

	_, err := fromEC2(context.Background(), "ip-hostname")
	assert.Error(t, err)

	ec2GetInstanceID = func(context.Context) (string, error) { return "someHostname", nil }

	hostname, err := fromEC2(context.Background(), "ip-hostname")
	assert.NoError(t, err)
	assert.Equal(t, "someHostname", hostname)
}

func TestFromEc2Prioritize(t *testing.T) {
	// This test than when a NON default EC2 hostname is detected but ec2_prioritize_instance_id_as_hostname is set
	// to true we use the instance ID
	defer func() { ec2GetInstanceID = ec2.GetInstanceID }()
	config.Mock(t)
	config.Datadog.SetWithoutSource("ec2_prioritize_instance_id_as_hostname", true)

	// make AWS provider return an error
	ec2GetInstanceID = func(context.Context) (string, error) { return "", fmt.Errorf("some error") }

	_, err := fromEC2(context.Background(), "non-default-hostname")
	assert.Error(t, err)

	ec2GetInstanceID = func(context.Context) (string, error) { return "someHostname", nil }

	hostname, err := fromEC2(context.Background(), "non-default-hostname")
	assert.NoError(t, err)
	assert.Equal(t, "someHostname", hostname)
}
