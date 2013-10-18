package ipaddress

import (
	. "github.com/r7kamura/gospel"
	"testing"
)

func TestHostname(t *testing.T) {
	Describe(t, "ipaddress.Name", func() {
		collector := &IpAddress{}

		It("should have its own name", func() {
			Expect(collector.Name()).To(Equal, "ipaddress")
		})
	})

	Describe(t, "verity.Collector.IpAddress.Collect", func() {
		collector := &IpAddress{}
		result, _ := collector.Collect()
		ipaddress := "10.0.2.15"

		It("should be able to collect ipaddress", func() {
			Expect(result.(string)).To(Equal, ipaddress)
		})
	})
}
