package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatSsiProcesses(t *testing.T) {
	processes := []InjectedProcess{{
		Pid:             1,
		ServiceName:     "abc",
		LanguageName:    "python",
		RuntimeName:     "cpython",
		RuntimeVersion:  "3.12.1",
		LibraryVersion:  "2.12.1",
		InjectorVersion: "0.20.1",
		IsInjected:      true,
		InjectionStatus: "complete",
		Reason:          "",
	}}
	out := FormatInjectedProcesss(processes)

	expected := "" +
		"  PID  SERVICE NAME  LANGUAGE NAME  RUNTIME NAME  RUNTIME VERSION  LIBRARY VERSION  INJECTOR VERSION  IS INJECTED  INJECTION STATUS  REASON  \n" +
		"  1    abc           python         cpython       3.12.1           2.12.1           0.20.1            true         complete                  \n"
	assert.Equal(t, expected, out)
}
