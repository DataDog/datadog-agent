package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
)

type stressEventHandler struct {
	count    int
	filename string
}

func (h *stressEventHandler) HandleEvent(event *sprobe.Event) {
	if event.GetType() == "open" {
		if flags := event.Open.Flags; flags&syscall.O_CREAT != 0 {
			filename, err := event.GetFieldValue("open.filename")
			if err == nil && filename.(string) == h.filename {
				h.count++
			}
		}
	}
}

func BenchmarkE2EOpen(b *testing.B) {
	rule := &policy.RuleDefinition{
		ID:         "test-rule",
		Expression: `open.filename == "{{.Root}}/test" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestProbe(nil, []*policy.RuleDefinition{rule})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("test")
	if err != nil {
		b.Fatal(err)
	}

	handler := &stressEventHandler{filename: testFile}
	test.probe.SetEventHandler(handler)
	test.probe.ResetStats()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		f, err := os.Create(testFile)
		if err != nil {
			b.Fatal(err)
		}

		if err := f.Close(); err != nil {
			b.Fatal(err)
		}
	}

	stats := test.probe.GetStats()
	b.ReportMetric(float64(stats.Events.Lost), "lost")
	b.ReportMetric(float64(handler.count), "events")
	b.ReportMetric(100*float64(handler.count)/float64(b.N), "%seen")
	b.ReportMetric(100*float64(stats.Events.Lost)/float64(b.N), "%lost")
}
