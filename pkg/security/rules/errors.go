package rules

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

var RuleWithoutEventErr = errors.New("rule without event")

var RuleWithMultipleEventsErr = errors.New("rule with multiple events")

type FieldTypeUnknown struct {
	Field string
}

func (e *FieldTypeUnknown) Error() string {
	return fmt.Sprintf("field type unknown for `%s`", e.Field)
}

type ValueTypeUnknown struct {
	Field string
}

func (e *ValueTypeUnknown) Error() string {
	return fmt.Sprintf("value type unknown for `%s`", e.Field)
}

type NoApprover struct {
	Fields []string
}

func (e NoApprover) Error() string {
	return fmt.Sprintf("no approver for fields `%s`", strings.Join(e.Fields, ", "))
}

type DuplicateRuleID struct {
	ID string
}

func (e DuplicateRuleID) Error() string {
	return fmt.Sprintf("duplicate rule ID `%s`", e.ID)
}

type NoEventTypeBucket struct {
	EventType string
}

func (e NoEventTypeBucket) Error() string {
	return fmt.Sprintf("no bucket for event type `%s`", e.EventType)
}
