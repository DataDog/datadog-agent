package eval

import "fmt"

type CapabilityNotFound struct {
	Field string
}

func (e *CapabilityNotFound) Error() string {
	return fmt.Sprintf("capability not found for `%s`", e.Field)
}

type CapabilityMismatch struct {
	Field string
}

func (e *CapabilityMismatch) Error() string {
	return fmt.Sprintf("capability mismatch for `%s`", e.Field)
}

type ValueTypeUnknown struct {
	Field string
}

func (e *ValueTypeUnknown) Error() string {
	return fmt.Sprintf("value type unknown for `%s`", e.Field)
}

type NoValue struct {
	Field string
}

func (e *NoValue) Error() string {
	return fmt.Sprintf("no value for `%s`", e.Field)
}
