package dataobs

import (
	"fmt"

	"github.com/xeipuuv/gojsonschema"
)

type OpenLineage struct{}

func NewOpenLineage() *OpenLineage {
	return &OpenLineage{}
}

var schemaLoader gojsonschema.JSONLoader = gojsonschema.NewReferenceLoader("https://openlineage.io/spec/2-0-2/OpenLineage.json")

func (openLineage *OpenLineage) EventToLog(event []byte) (string, error) {
	fmt.Println("new event ", string(event))
	result, err := gojsonschema.Validate(schemaLoader, gojsonschema.NewBytesLoader(event))

	if result.Valid() {
		fmt.Printf("The OL event is valid\n")
		return string(event), nil
	} else {
		fmt.Printf("The OL event is not valid. see errors :\n")
		for _, desc := range result.Errors() {
			fmt.Printf("- %s\n", desc)
		}
		return "", err
	}
}
