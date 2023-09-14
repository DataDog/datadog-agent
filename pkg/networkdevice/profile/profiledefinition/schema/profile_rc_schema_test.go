package schema

import (
	"encoding/json"
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test_DeviceProfileRcConfigJsonSchema(t *testing.T) {
	// language=json
	instanceJson := `{
	"profile_definition": {
		"name": "my-profile"
	}
}`

	err := assertAgainstSchema(t, instanceJson)

	fmt.Printf("%#v\n", err) // using %#v prints errors hierarchy
	require.NoError(t, err)
}

func Test_Schema_TextCases(t *testing.T) {
	var testcases []string
	err := filepath.WalkDir("./schema_testcases", func(s string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if filepath.Ext(d.Name()) == ".json" {
			testcases = append(testcases, s)
		}
		return nil
	})
	require.NoError(t, err)

	for _, testcaseJsonPath := range testcases {
		content, err := os.ReadFile(testcaseJsonPath)
		require.NoError(t, err)

		validationErr := assertAgainstSchema(t, string(content))
		validationErrStr := fmt.Sprintf("%#v\n", validationErr) // using %#v prints errors hierarchy

		testcaseExpectedErrPath := strings.ReplaceAll(testcaseJsonPath, ".json", "_expected_err.txt")
		testcaseExpectedErr, err := os.ReadFile(testcaseExpectedErrPath)
		require.NoError(t, err)

		assert.Equal(t, string(testcaseExpectedErr), validationErrStr)
	}
}

func assertAgainstSchema(t *testing.T, instanceJson string) error {
	sch, err := jsonschema.CompileString("schema.json", string(deviceProfileRcConfigJsonschema))
	require.NoError(t, err)

	var v interface{}
	err = json.Unmarshal([]byte(instanceJson), &v)
	require.NoError(t, err)

	err = sch.Validate(v)
	return err
}
