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

//go:embed etc/node_server.js
var nodeProg string

func (s *languageDetectionSuite) installNode() {
	s.Env().VM.Execute(
		"curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash - && " +
			"sudo apt-get install -y nodejs")
	nodeVersion := s.Env().VM.Execute("node --version")
	require.True(s.T(), strings.HasPrefix(nodeVersion, "v20."))
}

func (s *languageDetectionSuite) TestNodeDetection() {
	s.installNode()

	s.Env().VM.Execute(fmt.Sprintf(`echo "%s" > prog.js`, nodeProg))
	s.Env().VM.Execute("nohup node prog.js >myscript.log 2>&1 </dev/null &")

	s.checkDetectedLanguage("node", "node")
}
