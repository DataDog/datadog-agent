// This program is used to test the retrieval of the hostname from an external
// binary. It prints TRACE_TEST_HOSTNAME_RESPONSE and exits with
// TRACE_TEST_HOSTNAME_EXIT_CODE, so a single compiled binary can be reused
// across TestConfigHostname/external's subtests instead of recompiling one
// per subtest.

package main

import (
	"fmt"
	"os"
	"strconv"
)

func main() {
	fmt.Println(os.Getenv("TRACE_TEST_HOSTNAME_RESPONSE"))
	code, _ := strconv.Atoi(os.Getenv("TRACE_TEST_HOSTNAME_EXIT_CODE"))
	os.Exit(code)
}
