// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/stretchr/testify/require"
)

// nodeMajor is the major version of nodejs that will be installed
const nodeMajor = 20

//go:embed etc/node_server.js
var nodeProg string

func (s *languageDetectionSuite) installNode() {
	// Installation instructions taken from https://github.com/nodesource/distributions
	s.Env().RemoteHost.MustExecute("sudo apt-get update")
	s.Env().RemoteHost.MustExecute("sudo apt-get install -y curl")
	s.Env().RemoteHost.MustExecute("curl -fsSL https://nodejs.org/dist/v20.20.1/node-v20.20.1-linux-x64.tar.gz | sudo tar -xz -C /usr/local --strip-components=1")

	// Verify that node was installed correctly
	nodeVersion := s.Env().RemoteHost.MustExecute("node --version")
	require.True(s.T(), strings.HasPrefix(nodeVersion, fmt.Sprintf("v%d.", nodeMajor)))
}

func (s *languageDetectionSuite) TestNodeDetection() {
	s.installNode()

	s.Env().RemoteHost.MustExecute(fmt.Sprintf(`echo "%s" > prog.js`, nodeProg))
	pid := s.Env().RemoteHost.MustExecute("nohup node prog.js >myscript.log 2>&1 </dev/null & echo -n $!")

	s.checkDetectedLanguage(pid, "node", "process_collector")
}
