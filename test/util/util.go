package util

import (
	"time"

	"math/rand"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandomString(size int) string {
	b := make([]byte, size)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func TimeNowNano() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Second) // Unix time with nanosecond precision
}
