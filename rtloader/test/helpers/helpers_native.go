package helpers

/*
#cgo CFLAGS: -I../../include -I../../common
#cgo !windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -L../../rtloader/ -ldatadog-agent-rtloader -lstdc++ -static

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"

void TestMemoryTracker(void *, size_t, rtloader_mem_ops_t);
void initTestMemoryTracker(void) {
	set_memory_tracker_cb(TestMemoryTracker);
}

*/
import "C"

// InitMemoryTracker initializes RTLoader memory tracking
func InitMemoryTracker() {
	C.initTestMemoryTracker()
}
