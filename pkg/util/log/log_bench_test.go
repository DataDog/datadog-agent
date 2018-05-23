package log

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/cihub/seelog"
)

func BenchmarkVanilla(b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")

	for n := 0; n < b.N; n++ {
		l.Infof("this is a credential encoding urI: %s", "http://user:password@host:port")
	}
}

func BenchmarkScrubbing(b *testing.B) {
	var buff bytes.Buffer
	w := bufio.NewWriter(&buff)

	l, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	SetupDatadogLogger(l)

	for n := 0; n < b.N; n++ {
		Infof("this is a credential encoding urI: %s", "http://user:password@host:port")
	}
}

func BenchmarkScrubbingMulti(b *testing.B) {
	var buffA, buffB bytes.Buffer
	wA := bufio.NewWriter(&buffA)
	wB := bufio.NewWriter(&buffB)

	lA, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(wA, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	lB, _ := seelog.LoggerFromWriterWithMinLevelAndFormat(wB, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")

	SetupDatadogLogger(lA)
	_ = RegisterAdditionalLogger("extra", lB)

	Info("this is an API KEY: ", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	Infof("this is a credential encoding urI: %s", "http://user:password@host:port")

	for n := 0; n < b.N; n++ {
		Infof("this is a credential encoding urI: %s", "http://user:password@host:port")
	}
}
