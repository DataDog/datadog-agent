package main

type ErrorSecret struct {
	SecretId string
	Error    error
}

func (be *ErrorSecret) GetSecretOutput(secretKey string) SecretOutput {
	errorStr := be.Error.Error()
	return SecretOutput{
		Value: nil,
		Error: &errorStr,
	}
}
