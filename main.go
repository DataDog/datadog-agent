package main

/*
#cgo pkg-config: python-2.7
#include "checks.h"
*/
import "C"

func main() {
	C.get_checks(nil)
	C.run_check(C.CString("directory"))
}
