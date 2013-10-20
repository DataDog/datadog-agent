package network

import (
	. "github.com/r7kamura/gospel"
	"testing"
)

func TestNetwork(t *testing.T) {
	Describe(t, "network.Name", func() {
		collector := &Network{}

		It("should have its own name", func() {
			Expect(collector.Name()).To(Equal, "network")
		})
	})

	Describe(t, "network.Collect", func() {
		collector := &Network{}
		result, _ := collector.Collect()

		It("should have interfaces attribute", func() {
			_, ok := result.(map[string]interface{})["interfaces"]
			Expect(ok).To(Equal, true)
		})

		It("should have loopback interface info", func() {
			interfaces, _ := result.(map[string]interface{})["interfaces"]
			lo, ok := interfaces.(map[string]interface{})["lo"]
			Expect(ok).To(Equal, true)

			_, ok = lo.(map[string]interface{})["mtu"]
			Expect(ok).To(Equal, true)

			addresses, ok := lo.(map[string]interface{})["addresses"]
			Expect(ok).To(Equal, true)

			_, ok = addresses.(map[string]interface{})["127.0.0.1"]
			Expect(ok).To(Equal, true)
		})
	})
}
