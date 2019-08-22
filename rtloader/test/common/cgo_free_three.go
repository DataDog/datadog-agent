// +build three

package testcommon

/*
#cgo CFLAGS: -I../../common
#cgo !windows LDFLAGS: -L../../three/ -ldatadog-agent-three
#cgo windows LDFLAGS: -L../../three/ -ldatadog-agent-three.dll
#include "cgo_free.h"

extern void cgo_free(void *ptr);

void c_callCgoFree(void *ptr) {
	cgo_free(ptr);
}
*/
import "C"
