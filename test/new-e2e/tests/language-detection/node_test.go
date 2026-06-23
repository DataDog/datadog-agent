// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	_ "embed"
	"fmt"
)

//go:embed etc/node_server.js
var nodeProg string

func (s *languageDetectionSuite) TestNodeDetection() {
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(`echo "%s" > prog.js`, nodeProg))
	pid := s.Env().RemoteHost.MustExecute("nohup node prog.js >myscript.log 2>&1 </dev/null & echo -n $!")

	s.checkDetectedLanguage(pid, "node", "process_collector")
}
