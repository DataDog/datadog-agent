package common

import (
	"flag"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

var (
	fieldsVar string
	fields    []string // Hashing fields

	hashDeclared     bool
	hashDeclaredLock = &sync.Mutex{}
)

func HashFlag() {
	hashDeclaredLock.Lock()
	defer hashDeclaredLock.Unlock()

	if hashDeclared {
		return
	}
	hashDeclared = true
	flag.StringVar(&fieldsVar, "format.hash", "SamplerAddress", "List of fields to do hashing, separated by commas")

}

func ManualHashInit() error {
	fields = strings.Split(fieldsVar, ",")
	return nil
}

func HashProtoLocal(msg interface{}) string {
	return HashProto(fields, msg)
}

func HashProto(fields []string, msg interface{}) string {
	var keyStr string

	if msg != nil {
		vfm := reflect.ValueOf(msg)
		vfm = reflect.Indirect(vfm)

		for _, kf := range fields {
			fieldValue := vfm.FieldByName(kf)
			if fieldValue.IsValid() {
				keyStr += fmt.Sprintf("%v-", fieldValue)
			}
		}
	}

	return keyStr
}
