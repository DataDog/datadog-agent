package verity

import (
	. "github.com/r7kamura/gospel"
	"os"
	"testing"
)

func TestCollector(t *testing.T) {
	Describe(t, "Collect", func() {
		os.Setenv("VERITY_TEST", "1")

		result, _ := Collect()

		It("should be able to collect CPU info", func() {
			Expect(result["cpu"]).To(Exist)
		})

		It("should be able to collect env info", func() {
			Expect(result["test"]).To(Equal, "1")
		})

		It("should be able to collect hostname info", func() {
			Expect(result["hostname"]).To(Exist)
		})

		It("should be able to collect memory info", func() {
			Expect(result["memory"]).To(Exist)
		})
	})
}
