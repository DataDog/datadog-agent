// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package languagedetection

import (
	"strings"

	"github.com/stretchr/testify/require"
)

func (s *languageDetectionSuite) installPython() {
	s.Env().RemoteHost.MustExecute("sudo apt-get -y install python3")
	pyVersion := s.Env().RemoteHost.MustExecute("python3 --version")
	require.True(s.T(), strings.HasPrefix(pyVersion, "Python 3"))
}

func (s *languageDetectionSuite) TestPythonDetection() {
	s.installPython()

	s.Env().RemoteHost.MustExecute("echo 'import time\ntime.sleep(30)' > prog.py")
	s.Env().RemoteHost.MustExecute("nohup python3 prog.py >myscript.log 2>&1 </dev/null &")

	s.checkDetectedLanguage("python3", "python")
}
