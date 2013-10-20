package ipaddress

import (
	. "github.com/r7kamura/gospel"
	"testing"
)

func TestIpAddress(t *testing.T) {
	Describe(t, "ipaddress.Name", func() {
		collector := &IpAddress{}

		It("should have its own name", func() {
			Expect(collector.Name()).To(Equal, "ipaddress")
		})
	})
}
