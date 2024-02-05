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
	s.Env().RemoteHost.MustExecute("sudo apt-get install -y ca-certificates curl gnupg")
	s.Env().RemoteHost.MustExecute("sudo mkdir -p /etc/apt/keyrings")
	s.Env().RemoteHost.MustExecute("curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | sudo gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg")
	s.Env().RemoteHost.MustExecute(fmt.Sprintf("echo \"deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_%d.x nodistro main\" | sudo tee /etc/apt/sources.list.d/nodesource.list", nodeMajor))
	s.Env().RemoteHost.MustExecute("sudo apt-get update")
	s.Env().RemoteHost.MustExecute("sudo apt-get install nodejs -y")

	// Verify that node was installed correctly
	nodeVersion := s.Env().RemoteHost.MustExecute("node --version")
	require.True(s.T(), strings.HasPrefix(nodeVersion, fmt.Sprintf("v%d.", nodeMajor)))
}

func (s *languageDetectionSuite) TestNodeDetection() {
	s.installNode()

	s.Env().RemoteHost.MustExecute(fmt.Sprintf(`echo "%s" > prog.js`, nodeProg))
	s.Env().RemoteHost.MustExecute("nohup node prog.js >myscript.log 2>&1 </dev/null &")

	s.checkDetectedLanguage("node", "node")
}
