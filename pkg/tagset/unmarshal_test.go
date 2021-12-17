package tagset

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func ExampleUnmarshalBuilder_UnmarshalJSON() {
	u := UnmarshalBuilder{}
	err := json.Unmarshal([]byte(`["abc", "def"]`), &u)
	if err != nil {
		fmt.Printf("err: %s\n", err)
	}
	fmt.Printf("%#v", u.Tags.Sorted())
	// Output: []string{"abc", "def"}
}

func ExampleUnmarshalBuilder_UnmarshalJSON_nested() {
	type Metric struct {
		Name string
		Tags UnmarshalBuilder
	}

	metric := Metric{}
	err := json.Unmarshal([]byte(`{
		"Name": "met",
		"Tags":["abc", "def"]
	}`), &metric)
	if err != nil {
		fmt.Printf("err: %s\n", err)
		return
	}

	fmt.Printf("%#v\n", metric.Tags.Tags.Sorted())
	// Output: []string{"abc", "def"}
}

func ExampleUnmarshalBuilder_UnmarshalJSON_slice() {
	// note that it is not possible to provide a factory in this configuration
	tagsets := []UnmarshalBuilder{}
	err := json.Unmarshal([]byte(`[["xyz"],["abc", "def"]]`), &tagsets)
	if err != nil {
		fmt.Printf("err: %s\n", err)
		return
	}

	fmt.Printf("%#v\n", tagsets[0].Tags.Sorted())
	fmt.Printf("%#v\n", tagsets[1].Tags.Sorted())
	// Output:
	// []string{"xyz"}
	// []string{"abc", "def"}
}

func TestUnmarshalBuilder_JSON_factory(t *testing.T) {
	f, _ := NewCachingFactory(10, 5)
	u := UnmarshalBuilder{Factory: f}

	require.NoError(t, json.Unmarshal([]byte(`["abc", "def"]`), &u))
	t1 := u.Tags
	require.Equal(t, []string{"abc", "def"}, t1.Sorted())

	// unmarshal again to see that we get the same Tags instance
	require.NoError(t, json.Unmarshal([]byte(`["abc", "def"]`), &u))
	t2 := u.Tags
	require.Equal(t, []string{"abc", "def"}, t2.Sorted())

	require.True(t, t1 == t2)
}

func TestUnmarshalBuilder_JSON_error(t *testing.T) {
	u := UnmarshalBuilder{}

	require.Error(t, json.Unmarshal([]byte(`"abc"`), &u))
	require.Error(t, json.Unmarshal([]byte(`123`), &u))
	require.Error(t, json.Unmarshal([]byte(`{}`), &u))
	require.Error(t, json.Unmarshal([]byte(`true`), &u))
	require.Error(t, json.Unmarshal([]byte(`false`), &u))
	require.Error(t, json.Unmarshal([]byte(`[1, 2]`), &u))
	require.Error(t, json.Unmarshal([]byte(`null`), &u))
}

func ExampleUnmarshalBuilder_UnmarshalYAML() {
	u := UnmarshalBuilder{}
	err := yaml.Unmarshal([]byte(`
- "abc"
- def`), &u)
	if err != nil {
		fmt.Printf("err: %s\n", err)
	}
	fmt.Printf("%#v", u.Tags.Sorted())
	// Output: []string{"abc", "def"}
}

func ExampleUnmarshalBuilder_null_surprise() {
	u := UnmarshalBuilder{}

	// use the UnmarshalBuilder to unmarshal a "normal" value
	err := yaml.Unmarshal([]byte(`["abc"]`), &u)
	if err != nil {
		fmt.Printf("err: %s\n", err)
		return
	}
	fmt.Printf("%#v\n", u.Tags.Sorted())

	// re-use the same UnmarshalBuilder to unmarshal a null value
	err = yaml.Unmarshal([]byte(`["abc"]`), &u)
	if err != nil {
		fmt.Printf("err: %s\n", err)
		return
	}
	// surprisingly, the existing value of u.Tags is not overwritten!
	fmt.Printf("%#v\n", u.Tags.Sorted())

	// Output:
	// []string{"abc"}
	// []string{"abc"}
}

func ExampleUnmarshalBuilder_UnmarshalYAML_nested() {
	m := map[string]UnmarshalBuilder{}
	err := yaml.Unmarshal([]byte(`
tagset1:
  - "abc"
tagset2: []
# empty values (nulls) are treated as nil
tagset3:
`), &m)
	if err != nil {
		fmt.Printf("err: %s\n", err)
	}
	for _, k := range []string{"tagset1", "tagset2", "tagset3"} {
		v := m[k]
		if v.Tags != nil {
			fmt.Printf("%s: %#v\n", k, v.Tags.Sorted())
		} else {
			fmt.Printf("%s: nil\n", k)
		}
	}
	// Output:
	// tagset1: []string{"abc"}
	// tagset2: []string{}
	// tagset3: nil
}

func TestUnmarshalBuilder_JSON_round_trip(t *testing.T) {
	f, _ := NewCachingFactory(10, 5)

	t1 := f.NewTags([]string{"abc", "def"})
	j, err := json.Marshal(t1)
	require.NoError(t, err)

	u := UnmarshalBuilder{Factory: f}
	require.NoError(t, json.Unmarshal(j, &u))
	t2 := u.Tags

	// round-tripping should have found the same Tags instance
	require.True(t, t1 == t2)
}

func TestUnmarshalBuilder_YAML_round_trip(t *testing.T) {
	f, _ := NewCachingFactory(10, 5)

	t1 := f.NewTags([]string{"abc", "def"})
	j, err := yaml.Marshal(t1)
	require.NoError(t, err)

	u := UnmarshalBuilder{Factory: f}
	require.NoError(t, yaml.Unmarshal(j, &u))
	t2 := u.Tags

	// round-tripping should have found the same Tags instance
	require.True(t, t1 == t2)
}

func TestUnmarshalBuilder_YAML_empty(t *testing.T) {
	f, _ := NewCachingFactory(10, 5)

	u := UnmarshalBuilder{Factory: f}
	require.NoError(t, yaml.Unmarshal([]byte("[]"), &u))

	require.Equal(t, []string{}, u.Tags.Sorted())
}

func TestUnmarshalBuilder_YAML_null(t *testing.T) {
	f, _ := NewCachingFactory(10, 5)

	u := UnmarshalBuilder{Factory: f}
	require.NoError(t, yaml.Unmarshal([]byte("null"), &u))

	require.Nil(t, u.Tags)
}

func TestUnmarshalBuilder_YAML_null_nested(t *testing.T) {
	m := map[string]UnmarshalBuilder{}
	require.NoError(t, yaml.Unmarshal([]byte("a:\nb:\n"), &m))

	require.Nil(t, m["a"].Tags)
}
