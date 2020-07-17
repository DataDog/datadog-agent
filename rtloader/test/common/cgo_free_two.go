// +build two

package testcommon

/*
#cgo !windows LDFLAGS: -L../../two/ -ldatadog-agent-two
#cgo windows LDFLAGS: -L../../two/ -ldatadog-agent-two.dll
#include "cgo_free.h"

void c_callCgoFree(void *ptr) {
	cgo_free(ptr);
}
*/
import "C"
