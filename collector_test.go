package verity

import (
	. "github.com/r7kamura/gospel"
	"testing"
)

func TestCollector(t *testing.T) {
	Describe(t, "Collect", func() {
		result, _ := Collect()

		It("should be able to collect hostname info", func() {
			Expect(result["hostname"]).To(Exist)
		})

		It("should be able to collect CPU info", func() {
			Expect(result["cpu"]).To(Exist)
		})

		It("should be able to collect memory info", func() {
			Expect(result["memory"]).To(Exist)
		})
	})
}
