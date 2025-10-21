package common

func ApplyOption[T any](instance *T, options []func(*T) error) (*T, error) {
	for _, o := range options {
		if err := o(instance); err != nil {
			return nil, err
		}
	}
	return instance, nil
}
