// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package updater contains tests for the updater package
package updater

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// installDocker installs docker on the host
func installDocker(distro os.Descriptor, t *testing.T, host *components.RemoteHost) {
	switch distro {
	case os.UbuntuDefault:
		_, err := host.WriteFile("/tmp/install-docker.sh", []byte(`
			sudo apt-get update
			sudo apt-get install ca-certificates curl
			sudo install -m 0755 -d /etc/apt/keyrings
			sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
			sudo chmod a+r /etc/apt/keyrings/docker.asc
			echo \
			  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu \
			  $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
			  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
			sudo apt-get update
			sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
		`))
		require.Nil(t, err)
		host.MustExecute(`sudo chmod +x /tmp/install-docker.sh`)
		host.MustExecute(`sudo /tmp/install-docker.sh`)
		err = host.Remove("/tmp/install-docker.sh")
		require.Nil(t, err)
	case os.CentOSDefault:
		_, err := host.WriteFile("/tmp/install-docker.sh", []byte(`
			sudo yum install -y yum-utils
			sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
			sudo yum install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
			sudo systemctl start docker
		`))
		require.Nil(t, err)
		host.MustExecute(`sudo chmod +x /tmp/install-docker.sh`)
		host.MustExecute(`sudo /tmp/install-docker.sh`)
		err = host.Remove("/tmp/install-docker.sh")
		require.Nil(t, err)
	default:
		t.Fatalf("unsupported distro: %s", distro.String())
	}
}

// launchJavaDockerContainer launches a small Java HTTP server in a docker container
func launchJavaDockerContainer(t *testing.T, host *components.RemoteHost) {
	host.MustExecute(`sudo docker run -d -p8887:8888 baptistefoy702/message-server:latest`)
	assert.Eventually(t,
		func() bool {
			_, err := host.Execute(`curl -m 1 localhost:8887/messages`) // Generate a trace
			return err == nil
		}, 10*time.Second, 100*time.Millisecond,
	)

}
