package ipv6address

import (
	. "github.com/r7kamura/gospel"
	"testing"
)

func TestIpv6Address(t *testing.T) {
	Describe(t, "ipv6address.Name", func() {
		collector := &Ipv6Address{}

		It("should have its own name", func() {
			Expect(collector.Name()).To(Equal, "ipv6address")
		})
	})
}
