package eval

// Model - interface that a model has to implement for the rule compilation
type Model interface {
	// GetEvaluator - Returns an evaluator for the given field
	GetEvaluator(field Field) (interface{}, error)
	// ValidateField - Returns whether the value use against the field is valid, ex: for constant
	ValidateField(field Field, value FieldValue) error
	// SetEvent - Set the current object instance for this model
	SetEvent(event interface{})
	// GetEvent - Returns the current object instance for this model
	GetEvent() Event
}
