package utils

// Value represents either an error or an actual value of type T.
//
// The default value of the type is the default value of T (no error).
type Value[T any] struct {
	value T
	err   error
}

// NewValue initializes a Value[T] with the given value of type T and no error.
func NewValue[T any](value T) Value[T] {
	return Value[T]{
		value: value,
	}
}

// NewErrorValue initializes a Value[T] with the given error.
//
// Note that if err is nil, the returned Value[T] is fundamentaly equivalent to a Value[T]
// containing the default value of T and no error.
func NewErrorValue[T any](err error) Value[T] {
	return Value[T]{
		err: err,
	}
}

// NewErrorValue initializes a Value[T] with the given error.
//
// Note that if err is nil, the returned Value[T] is fundamentaly equivalent to a Value[T]
// containing the default value of T and no error.
//
// This function is equivalent to NewErrorValue[T] but allows not to specify the value of T explicitely
func (Value[T]) NewErrorValue(err error) Value[T] {
	return NewErrorValue[T](err)
}

// Value returns the value and error stored in the Value[T].
//
// If the Value[T] represents an error, it returns the default value of type T
// and a non-nil error, otherwise the stored value of type T and a nil error.
func (value *Value[T]) Value() (T, error) {
	return value.value, value.err
}
