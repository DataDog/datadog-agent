package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFilePath(t *testing.T) {
	tests := []struct {
		input  string
		err    bool
		output PathMeta
	}{
		{
			input:  "datadog/2/APM_SAMPLING/fc18c18f-939a-4017-b428-af03678f6c1a/file1",
			err:    false,
			output: PathMeta{Type: TypeDatadog, OrgID: 2, Product: ProductAPMSampling, ConfigID: "fc18c18f-939a-4017-b428-af03678f6c1a", Name: "file1"},
		},
		{
			input:  "user/5343/TESTING1/static_id/f3045934w_dogfile",
			err:    false,
			output: PathMeta{Type: TypeUnknown, OrgID: 5343, Product: ProductTesting1, ConfigID: "static_id", Name: "f3045934w_dogfile"},
		},
		{
			input: "user/5343/TESTING1/static_id/f3045934w_dogfile/other_file",
			err:   true,
		},
		{
			input: "user/a/TESTING1/static_id/f3045934w_dogfile",
			err:   true,
		},
		{
			input: "/5343/TESTING1/static_id/f3045934w_dogfile",
			err:   true,
		},
		{
			input: "user//TESTING1/static_id/f3045934w_dogfile",
			err:   true,
		},
		{
			input: "user/5343//static_id/f3045934w_dogfile",
			err:   true,
		},
		{
			input: "user/5343/TESTING1//f3045934w_dogfile",
			err:   true,
		},
		{
			input: "user/5343/TESTING1/static_id/",
			err:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.input, func(tt *testing.T) {
			output, err := ParseFilePathMeta(test.input)
			if test.err {
				assert.Error(tt, err)
			} else {
				assert.Equal(tt, test.output, output)
				assert.NoError(tt, err)
			}
		})
	}
}
