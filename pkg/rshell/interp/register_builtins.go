// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/awk"
	breakcmd "github.com/DataDog/datadog-agent/pkg/rshell/builtins/break"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/cat"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/cd"
	continuecmd "github.com/DataDog/datadog-agent/pkg/rshell/builtins/continue"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/cut"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/df"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/du"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/echo"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/exit"
	falsecmd "github.com/DataDog/datadog-agent/pkg/rshell/builtins/false"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/find"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/free"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/grep"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/head"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/help"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/ip"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/journalctl"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/jq"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/logrotate"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/ls"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/lsof"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/ping"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/pmap"
	printfcmd "github.com/DataDog/datadog-agent/pkg/rshell/builtins/printf"
	pscmd "github.com/DataDog/datadog-agent/pkg/rshell/builtins/ps"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/pwd"
	readcmd "github.com/DataDog/datadog-agent/pkg/rshell/builtins/read"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/rm"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/sed"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/sha256sum"
	sortcmd "github.com/DataDog/datadog-agent/pkg/rshell/builtins/sort"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/ss"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/stat"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/strings_cmd"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/systemctl"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/tail"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/tee"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testcmd"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/tr"
	truecmd "github.com/DataDog/datadog-agent/pkg/rshell/builtins/true"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/truncate"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/uname"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/uniq"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/uptime"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/vmstat"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/wc"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/xargs"
)

var registerOnce sync.Once

func registerBuiltins() {
	registerOnce.Do(func() {
		cmds := []builtins.Command{
			awk.Cmd,
			breakcmd.Cmd,
			cat.Cmd,
			cd.Cmd,
			cut.Cmd,
			continuecmd.Cmd,
			df.Cmd,
			du.Cmd,
			echo.Cmd,
			exit.Cmd,
			falsecmd.Cmd,
			find.Cmd,
			free.Cmd,
			grep.Cmd,
			head.Cmd,
			help.Cmd,
			ip.Cmd,
			journalctl.Cmd,
			jq.Cmd,
			logrotate.Cmd,
			ls.Cmd,
			lsof.Cmd,
			ping.Cmd,
			pmap.Cmd,
			sortcmd.Cmd,
			printfcmd.Cmd,
			pscmd.Cmd,
			pwd.Cmd,
			readcmd.Cmd,
			rm.Cmd,
			sed.Cmd,
			sha256sum.Cmd,
			ss.Cmd,
			stat.Cmd,
			strings_cmd.Cmd,
			systemctl.Cmd,
			tail.Cmd,
			tee.Cmd,
			testcmd.Cmd,
			testcmd.BracketCmd,
			tr.Cmd,
			truecmd.Cmd,
			truncate.Cmd,
			uname.Cmd,
			uniq.Cmd,
			uptime.Cmd,
			vmstat.Cmd,
			wc.Cmd,
			xargs.Cmd,
		}
		// Append platform-specific builtins (e.g. the Windows-only ntfs-du) so
		// they are registered — and thus listed by `help` and runnable — only on
		// platforms that support them.
		cmds = append(cmds, platformBuiltins()...)
		for _, cmd := range cmds {
			cmd.Register()
		}
	})
}
