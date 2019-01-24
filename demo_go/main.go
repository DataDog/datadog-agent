package main

// #cgo CFLAGS: -I${SRCDIR}/../include
// #cgo LDFLAGS: -L${SRCDIR}/../build/six -ldatadog-agent-six
// #include <datadog_agent_six.h>
import "C"
import "fmt"

func main() {
	py2 := C.make2()
	defer C.destroy2(py2)

	py3 := C.make3()
	defer C.destroy3(py3)

	fmt.Println(C.GoString(C.get_py_version(py2)))
	fmt.Println(C.GoString(C.get_py_version(py3)))
}
