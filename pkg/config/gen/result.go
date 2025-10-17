package gen

import (
	_ "embed"
	"fmt"
)

//go:embed fields.bin
var fieldsBinary []byte

func GetResult() []byte {
	fmt.Printf("*** GetResult, len(bin) = %d\n", len(fieldsBinary))
	return fieldsBinary
}
