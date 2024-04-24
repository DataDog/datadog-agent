// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package updater contains tests for the updater package
package updater

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// installDocker installs docker on the host
func installDocker(distro os.Descriptor, arch os.Architecture, t *testing.T, host *components.RemoteHost) {
	switch distro {
	case os.UbuntuDefault, os.DebianDefault:
		_, err := host.WriteFile(
			"/tmp/install-docker.sh",
			[]byte(
				fmt.Sprintf(`
sudo apt-get update
sudo apt-get install ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/%[1]s/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc
echo \
	"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/%[1]s \
	$(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
	sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin amazon-ecr-credential-helper

sudo mkdir -p /root/.docker
echo '{"credsStore": "ecr-login"}' | sudo tee /root/.docker/config.json
		`, distro.Flavor.String(),
				),
			),
		)
		require.Nil(t, err)
		host.MustExecute(`sudo chmod +x /tmp/install-docker.sh`)
		host.MustExecute(`sudo /tmp/install-docker.sh`)
		err = host.Remove("/tmp/install-docker.sh")
		require.Nil(t, err)
	case os.CentOSDefault, os.RedHatDefault:
		_, err := host.WriteFile(
			"/tmp/install-docker.sh",
			[]byte(
				fmt.Sprintf(`
sudo yum install -y yum-utils
sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
sudo yum install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl start docker

export previous_dir=$(pwd)
cd /usr/local
sudo curl https://dl.google.com/go/go1.22.1.linux-%s.tar.gz --output go.tar.gz
sudo tar -C /usr/local -xzf go.tar.gz
export PATH="$PATH:/usr/local/go/bin:$(go env GOPATH)/bin"
cd $previous_dir

go install github.com/awslabs/amazon-ecr-credential-helper/ecr-login/cli/docker-credential-ecr-login@latest

sudo mkdir -p /root/.docker
echo '{"credsStore": "ecr-login"}' | sudo tee /root/.docker/config.json
		`, string(arch),
				),
			),
		)
		require.Nil(t, err)
		host.MustExecute(`sudo chmod +x /tmp/install-docker.sh`)
		host.MustExecute(`sudo /tmp/install-docker.sh`)
		err = host.Remove("/tmp/install-docker.sh")
		require.Nil(t, err)
	case os.SuseDefault:
		_, err := host.WriteFile(
			"/tmp/install-docker.sh",
			[]byte(
				fmt.Sprintf(`
sudo zypper addrepo https://download.docker.com/linux/sles/docker-ce.repo
sudo zypper install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl start docker

export previous_dir=$(pwd)
cd /usr/local
sudo curl https://dl.google.com/go/go1.22.1.linux-%s.tar.gz --output go.tar.gz
sudo tar -C /usr/local -xzf go.tar.gz
export PATH="$PATH:/usr/local/go/bin"
export PATH="$PATH:$(go env GOPATH)/bin"
cd $previous_dir

go install github.com/awslabs/amazon-ecr-credential-helper/ecr-login/cli/docker-credential-ecr-login@latest

sudo mkdir -p /root/.docker
echo '{"credsStore": "ecr-login"}' | sudo tee /root/.docker/config.json
		`, string(arch),
				),
			),
		)
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
// and make a call to it
func launchJavaDockerContainer(t *testing.T, host *components.RemoteHost) {

	host.MustExecute(`sudo docker run -d -p8887:8888 669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/baptistefoy702/message-server:latest`)
	assert.Eventually(t,
		func() bool {
			_, err := host.Execute(`curl -m 1 localhost:8887/messages`)
			return err == nil
		}, 30*time.Second, 100*time.Millisecond,
	)
}
