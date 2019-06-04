package encoding

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
)

func TestAddressCaching(t *testing.T) {
	addr1 := util.AddressFromString("127.0.0.1")
	addr2 := util.AddressFromString("127.0.0.1")

	// Sanity check
	assert.NotEqual(t, stringptr(addr1.String()), stringptr(addr2.String()))

	// Now with string interning
	cache := make(AddrCache)
	str1 := cache.String(addr1)
	str2 := cache.String(addr2)
	assert.Equal(t, stringptr(str1), stringptr(str2))
	assert.Equal(t, "127.0.0.1", str1)
}

func stringptr(s string) uintptr {
	return (*reflect.StringHeader)(unsafe.Pointer(&s)).Data
}
