package macaddress

import (
	. "github.com/r7kamura/gospel"
	"testing"
)

func TestMacAddress(t *testing.T) {
	Describe(t, "macaddress.Name", func() {
		collector := &MacAddress{}

		It("should have its own name", func() {
			Expect(collector.Name()).To(Equal, "macaddress")
		})
	})
}
