package lib

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGitlabID(t *testing.T) {
	testCases := []struct {
		name     string
		data     []byte
		expected GitlabID
		err      error
	}{
		{
			name:     "int value",
			data:     []byte(`{"project_id": 123}`),
			expected: GitlabID("123"),
		},
		{
			name:     "str int value",
			data:     []byte(`{"project_id": "123"}`),
			expected: GitlabID("123"),
		},
		{
			name:     "group_name/project_name",
			data:     []byte(`{"project_id": "group_name/project_name"}`),
			expected: GitlabID("group_name/project_name"),
		},
		{
			name:     "empty value",
			data:     []byte(`{"project_id": ""}`),
			expected: GitlabID(""),
		},
		{
			name:     "null value",
			data:     []byte(`{"project_id": null}`),
			expected: GitlabID(""),
		},
		{
			name:     "boolean value",
			data:     []byte(`{"project_id": false}`),
			expected: GitlabID("group_name/project_name"),
			err:      fmt.Errorf("invalid gitlab ID: expecting string or number, got: false"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var got struct {
				ProjectID GitlabID `json:"project_id"`
			}
			err := json.Unmarshal(tc.data, &got)
			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, got.ProjectID)
			}
		})
	}
}
