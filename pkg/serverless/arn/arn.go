// +build serverless

package arn

import (
	"sync"
)

var currentARN struct {
	value string
	sync.Mutex
}

// BuildARN returns an ARN of the current running function.
func Get() string {
	currentARN.Lock()
	defer currentARN.Unlock()

	return currentARN.value
	//	region, exists := os.LookupEnv("AWS_REGION")
	//	if !exists {
	//		return "", fmt.Errorf("BuildARN: no region information available")
	//	}
	//	functionName, exists := os.LookupEnv("AWS_LAMBDA_FUNCTION_NAME")
	//	if !exists {
	//		return "", fmt.Errorf("BuildARN: no function information available")
	//	}
	//
	//	 arn:aws:lambda:us-east-1:123456789123:function:the-function-name
	//	return fmt.Sprintf("arn:aws:lambda:%s:123456789123:function:%s", region, functionName), nil
}

func Set(arn string) {
	currentARN.Lock()
	defer currentARN.Unlock()

	currentARN.value = arn
}
