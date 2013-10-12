package verity

import (
	. "github.com/r7kamura/gospel"
	"os"
	"testing"
)

func TestHostname(t *testing.T) {
	Describe(t, "verity.Collector.Hostname.Collect", func() {
		collector := &Hostname{}
		result, _ := collector.Collect()
		hostname, _ := os.Hostname()

		It("should be able to collect hostname", func() {
			Expect(result["hostname"]).To(Equal, hostname)
		})
	})
}
