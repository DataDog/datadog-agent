package network

import (
	. "github.com/r7kamura/gospel"
	"testing"
)

func TestNetwork(t *testing.T) {
	Describe(t, "network.Name", func() {
		collector := &IpAddress{}

		It("should have its own name", func() {
			Expect(collector.Name()).To(Equal, "network")
		})
	})
}
