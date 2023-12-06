package languagedetection

import (
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test00EnsureSetup ensures that the test environment is set up prior to running the language detection
// tests. Because tests in testify suites always run in order, this function is named such that it
// will run first.
func (s *languageDetectionSuite) Test00EnsureSetup() {
	s.EventuallyWithT(func(collect *assert.CollectT) {
		status := s.Env().VM.Execute(("sudo datadog-agent status"))
		assert.Contains(collect, status, "Agent start", "agent failed to return status")

		wl := s.Env().VM.Execute("sudo /opt/datadog-agent/bin/agent/agent workload-list")
		assert.NotEmpty(collect, strings.TrimSpace(wl), "agent workload-list was empty")
	}, 5*time.Minute, 10*time.Second, "setup never completed")
}
