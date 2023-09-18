package schema

import (
	"fmt"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestValidation(t *testing.T) {
	sch, err := jsonschema.CompileString("schema.json", string(DeviceProfileRcConfigJsonschema))
	require.NoError(t, err)

	// Check no additional / unevaluated property is allowed
	errAdditionalProperties := testAdditionalProperties(sch, make(map[*jsonschema.Schema]map[bool]bool), false)
	assert.NoError(t, errAdditionalProperties, "additionalProperties not allowed")
}

// testAdditionalProperties checks that the schema does not allow additional properties
// by setting either `additionalProperties: false` or `unevaluatedProperties: false`
func testAdditionalProperties(schema *jsonschema.Schema, visited map[*jsonschema.Schema]map[bool]bool, parentIsCond bool) error {
	// Current schema can be called from multiple parents, some of which could be conditions or not (see below),
	// which changes the behavior of unevaluatedProperties.
	// So we check the parent state in the "visited" map to make sure every case is checked.
	if schema == nil || visited[schema][parentIsCond] {
		return nil
	}
	if _, ok := visited[schema]; !ok {
		visited[schema] = map[bool]bool{}
	}
	visited[schema][parentIsCond] = true

	if schema.Ref != nil {
		// We can forward parentIsCond in that case, it's the only thing schema.Ref does
		return testAdditionalProperties(schema.Ref, visited, parentIsCond)
	}

	// As we can inherit types & properties **in conditions only**, we add a parentIsCond
	// field to skip validation when no type is defined and there are properties in a condition
	// Using conditions with `additionalProperties: false` sometimes render the condition useless,
	// so we need to be able to skip it
	if len(schema.Types) == 0 && !parentIsCond && len(schema.Properties) != 0 {
		return fmt.Errorf("%s object has no type", schema.String())
	}

	for _, objectType := range schema.Types {
		if objectType == "object" {
			// Same as the comment on inheritance, but this is the case where the type
			// is explicitely defined so we skip it
			if parentIsCond {
				continue
			}

			// allow additional properties if they are declared in sub schemas with
			// unevaluatedProperties: false
			if schema.UnevaluatedProperties != nil && !*schema.UnevaluatedProperties.Always {
				continue
			}

			err := fmt.Errorf("%s allows additional properties", schema.String())
			if schema.AdditionalProperties == nil {
				return err
			}
			if schema.AdditionalProperties.(bool) {
				return err
			}
		} else if objectType == "array" {
			if schema.Items2020 != nil {
				if err := testAdditionalProperties(schema.Items2020, visited, true); err != nil {
					return err
				}
			} else {
				switch it := schema.Items.(type) {
				case *jsonschema.Schema:
					if err := testAdditionalProperties(it, visited, false); err != nil {
						return err
					}
				case []*jsonschema.Schema:
					for _, sch := range it {
						if err := testAdditionalProperties(sch, visited, false); err != nil {
							return err
						}
					}
				case nil:
					continue
				default:
					return fmt.Errorf("Unknown jsonschema item")
				}
			}

			if schema.AdditionalItems != nil && !schema.AdditionalItems.(bool) {
				return fmt.Errorf("%s allows additional items", schema.String())
			}
		}
	}

	// Test sub-structures
	if err := testAdditionalProperties(schema.Not, visited, true); err != nil {
		return err
	}
	if err := testAdditionalProperties(schema.If, visited, true); err != nil {
		return err
	}
	if err := testAdditionalProperties(schema.Then, visited, true); err != nil {
		return err
	}
	if err := testAdditionalProperties(schema.Else, visited, true); err != nil {
		return err
	}
	for _, sch := range schema.AnyOf {
		if err := testAdditionalProperties(sch, visited, true); err != nil {
			return err
		}
	}
	for _, sch := range schema.AllOf {
		if err := testAdditionalProperties(sch, visited, true); err != nil {
			return err
		}
	}
	for _, sch := range schema.OneOf {
		if err := testAdditionalProperties(sch, visited, true); err != nil {
			return err
		}
	}

	// Properties
	if err := testAdditionalProperties(schema.Contains, visited, false); err != nil {
		return err
	}
	if err := testAdditionalProperties(schema.PropertyNames, visited, false); err != nil {
		return err
	}
	for _, sch := range schema.Properties {
		if err := testAdditionalProperties(sch, visited, false); err != nil {
			return err
		}
	}
	for _, sch := range schema.PatternProperties {
		if err := testAdditionalProperties(sch, visited, false); err != nil {
			return err
		}
	}
	return nil
}
