package util

type LogSync interface {
	Add(nbMessageReceived uint32)
}
