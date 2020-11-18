package logutil

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func TestThrottled(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Run("basic", func(t *testing.T) {
		l := NewThrottled(2, 10*time.Millisecond)
		var out bytes.Buffer
		logFunc := func(format string, params ...interface{}) error {
			out.WriteString(fmt.Sprintf(format, params...))
			return nil
		}
		for i := 0; i < 10; i++ {
			l.log(logFunc, "%d\n", i)
		}
		time.Sleep(20 * time.Millisecond) // reset
		for i := 10; i < 20; i++ {
			l.log(logFunc, "%d\n", i)
		}
		assert.Equal(t, "0\n1\nToo many similar messages, pausing up to 10ms...10\n11\nToo many similar messages, pausing up to 10ms...", out.String())
	})

	t.Run("resets", func(t *testing.T) {
		l := NewThrottled(2, 10*time.Millisecond)
		var out bytes.Buffer
		logFunc := func(format string, params ...interface{}) error {
			out.WriteString(fmt.Sprintf(format, params...))
			return nil
		}
		l.log(logFunc, "1\n")
		time.Sleep(20 * time.Millisecond) // reset
		l.log(logFunc, "2\n")
		l.log(logFunc, "3\n")
		time.Sleep(20 * time.Millisecond) // reset
		l.log(logFunc, "4\n")
		l.log(logFunc, "5\n")
		l.log(logFunc, "6\n")
		assert.Equal(t, "1\n2\n3\n4\n5\nToo many similar messages, pausing up to 10ms...", out.String())
	})

	t.Run("io.Writer", func(t *testing.T) {
		var out bytes.Buffer
		logger, err := seelog.LoggerFromWriterWithMinLevelAndFormat(&out, seelog.WarnLvl, "[%Level] %Msg")
		if err != nil {
			t.Fatal(err)
		}
		if err := seelog.ReplaceLogger(logger); err != nil {
			t.Fatal(err)
		}
		log.SetupLogger(logger, "INFO")
		l := NewThrottled(2, 10*time.Millisecond)
		l.Write([]byte("1\n"))
		time.Sleep(20 * time.Millisecond) // reset
		l.Write([]byte("2\n"))
		l.Write([]byte("3\n"))
		time.Sleep(20 * time.Millisecond) // reset
		l.Write([]byte("4\n"))
		l.Write([]byte("5\n"))
		l.Write([]byte("6\n"))
		l.Write([]byte("7\n"))
		l.Write([]byte("8\n"))
		l.Write([]byte("9\n"))
		logger.Flush()
		assert.Equal(t, "[Error] 1[Error] 2[Error] 3[Error] 4[Error] 5[Error] Too many similar messages, pausing up to 10ms...", out.String())
	})
}
