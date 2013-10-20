package env

import (
	. "github.com/r7kamura/gospel"
	"os"
	"testing"
)

func TestHostname(t *testing.T) {
	Describe(t, "env.Name()", func() {
		collector := &Env{}

		It("should have its own name", func() {
			Expect(collector.Name()).To(Equal, "env")
		})
	})

	Describe(t, "env.Collect()", func() {
		os.Setenv("VERITY_TEST1", "1")
		os.Setenv("VERITY_TEST2", "2")

		collector := &Env{}
		result, _ := collector.Collect()

		It("should be able to collect hostname", func() {
			Expect(result.(map[string]string)["test1"]).To(Equal, "1")
			Expect(result.(map[string]string)["test2"]).To(Equal, "2")
		})
	})
}
