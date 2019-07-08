// +build two

package testcommon

/*
#cgo CFLAGS: -I../../common
#cgo !windows LDFLAGS: -L../../two/ -ldatadog-agent-two
#cgo windows LDFLAGS: -L../../two/ -ldatadog-agent-two.dll
#include <memory.h>

void c_callCgoFree(void *ptr) {
	cgo_free(ptr);
}
*/
import "C"
