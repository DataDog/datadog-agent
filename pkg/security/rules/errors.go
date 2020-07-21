package rules

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

// ErrRuleWithoutEvent is returned when no event type was inferred from the rule
var ErrRuleWithoutEvent = errors.New("rule without event")

// ErrRuleWithMultipleEvents is returned when multiple event type were inferred from the rule
var ErrRuleWithMultipleEvents = errors.New("rule with multiple events")

// ErrFieldTypeUnknown is returned when a field has an unknown type
type ErrFieldTypeUnknown struct {
	Field string
}

func (e *ErrFieldTypeUnknown) Error() string {
	return fmt.Sprintf("field type unknown for `%s`", e.Field)
}

// ErrValueTypeUnknown is returned when the value of a field has an unknown type
type ErrValueTypeUnknown struct {
	Field string
}

func (e *ErrValueTypeUnknown) Error() string {
	return fmt.Sprintf("value type unknown for `%s`", e.Field)
}

// ErrNoApprover is returned when no approver was found for a set of rules
type ErrNoApprover struct {
	Fields []string
}

func (e ErrNoApprover) Error() string {
	return fmt.Sprintf("no approver for fields `%s`", strings.Join(e.Fields, ", "))
}

// ErrDuplicateRuleID is returned when 2 rules have the same identifier
type ErrDuplicateRuleID struct {
	ID string
}

func (e ErrDuplicateRuleID) Error() string {
	return fmt.Sprintf("duplicate rule ID `%s`", e.ID)
}

// ErrNoEventTypeBucket is returned when no bucket could be found for an event type
type ErrNoEventTypeBucket struct {
	EventType string
}

func (e ErrNoEventTypeBucket) Error() string {
	return fmt.Sprintf("no bucket for event type `%s`", e.EventType)
}
