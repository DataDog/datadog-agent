package hostname

import (
	. "github.com/r7kamura/gospel"
	"os"
	"testing"
)

func TestHostname(t *testing.T) {
	Describe(t, "hostname.Name", func() {
		collector := &Hostname{}

		It("should have its own name", func() {
			Expect(collector.Name()).To(Equal, "hostname")
		})
	})

	Describe(t, "verity.Collector.Hostname.Collect", func() {
		collector := &Hostname{}
		result, _ := collector.Collect()
		hostname, _ := os.Hostname()

		It("should be able to collect hostname", func() {
			Expect(result.(string)).To(Equal, hostname)
		})
	})
}
