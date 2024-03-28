// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type cmdTestCase struct {
	command           string
	expected          string
	obfuscatedIndices string
}

func TestBasicShellCommandObfuscation(t *testing.T) {
	tests := []cmdTestCase{
		{
			command:           "foo --pass secret not_secret --token=secret; md5 --password=pony; cat passwords.txt > /tmp/hello",
			expected:          "foo --pass ? not_secret --token=?; md5 ?; cat passwords.txt > /tmp/hello",
			obfuscatedIndices: "11:6 32:6 39:15",
		},
		{
			command:           "foo --pass '$(\"md5\" --arg)'",
			expected:          "foo --pass ?",
			obfuscatedIndices: "11:16",
		},
		{
			command:           "md5 --password=",
			expected:          "md5 ?",
			obfuscatedIndices: "4:11",
		},
		{
			command:           "md5 hello > /tmp/hello",
			expected:          "md5 ? > /tmp/hello",
			obfuscatedIndices: "4:5",
		},
		{
			command:           "md5 --password=pony",
			expected:          "md5 ?",
			obfuscatedIndices: "4:15",
		},
		{
			command:           "md5 --pass",
			expected:          "md5 ?",
			obfuscatedIndices: "4:6",
		},
		{
			command:           "cat passwords.txt other | while read line; do; md5 -s $line; done",
			expected:          "cat passwords.txt other | while read line; do; md5 ? ?; done",
			obfuscatedIndices: "51:2 53:5",
		},
		{
			command:           "cmd --pass abc --token=def",
			expected:          "cmd --pass ? --token=?",
			obfuscatedIndices: "11:3 21:3",
		},
		{
			command:           "cmd --pass",
			expected:          "cmd --pass",
			obfuscatedIndices: "",
		},
		{
			command:           "ENV=\"i'm a var env who say: $hello\" ENV2=\"ZZZ\" LD_PRELOAD=YYY ls",
			expected:          "ENV=? ENV2=? LD_PRELOAD=YYY ls",
			obfuscatedIndices: "4:31 11:5",
		},
		{
			command:           "ENV=XXX LD_PRELOAD=YYY ls",
			expected:          "ENV=? LD_PRELOAD=YYY ls",
			obfuscatedIndices: "4:3",
		},
		{
			command:           "ENV=$hey LD_PRELOAD=YYY ls",
			expected:          "ENV=? LD_PRELOAD=YYY ls",
			obfuscatedIndices: "4:4",
		},
		{
			command:           "ENV=$hey ENV2=$other LD_PRELOAD=YYY ENV3='quoted string' ls",
			expected:          "ENV=? ENV2=? LD_PRELOAD=YYY ENV3=? ls",
			obfuscatedIndices: "4:4 11:6 33:15",
		},
		{
			command:           "md5 --pass=pony",
			expected:          "md5 ?",
			obfuscatedIndices: "4:11",
		},
		{
			command:           "md5 --pass pony hash",
			expected:          "md5 ? ? ?",
			obfuscatedIndices: "4:6 6:4 8:4",
		},
		{
			command:           "md5 --pass=pony --pass pony",
			expected:          "md5 ? ? ?",
			obfuscatedIndices: "4:11 6:6 8:4",
		},
		{
			command:           "md5 -s pony",
			expected:          "md5 ? ?",
			obfuscatedIndices: "4:2 6:4",
		},
		{
			command:           "cmd --token; cmd --pass=x ; LD_PRELOAD=$token cmd2 hello world",
			expected:          "cmd --token; cmd --pass=? ; LD_PRELOAD=$token cmd2 hello world",
			obfuscatedIndices: "24:1",
		},

		// Subcommands tests
		{
			command:           "for i in `seq 1 5`; do echo \"Iteration: $i\" --pass=pony; done",
			expected:          "for i in `seq 1 5`; do echo \"Iteration: $i\" --pass=?; done",
			obfuscatedIndices: "51:4",
		},
		{
			command:           "echo hello `",
			expected:          "echo hello `",
			obfuscatedIndices: "",
		},
		{
			command:           "echo `cmd hello --pass secret",
			expected:          "echo `cmd hello --pass ?",
			obfuscatedIndices: "23:6",
		},
		{
			command:           "echo `cmd hello --pass secret`",
			expected:          "echo `cmd hello --pass ?`",
			obfuscatedIndices: "23:6",
		},
		{
			command:           "echo `cmd hello` --pass secret",
			expected:          "echo `cmd hello` --pass ?",
			obfuscatedIndices: "24:6",
		},
		{
			command:           "echo `md5 hello",
			expected:          "echo `md5 ?",
			obfuscatedIndices: "10:5",
		},
		{
			command:           "echo `md5 hello`",
			expected:          "echo `md5 ?`",
			obfuscatedIndices: "10:5",
		},
		/*
			The current implementation doesn't support nested subcommands of backticks
			{
				command:           "echo `md5 hello \\`ls\\``",
				expected:          "echo `md5 ? ?`",
				obfuscatedIndices: "10:5 12:4",
			},
		*/
		{
			command:           "cmd --pass $(md5 file1 file2 file3)",
			expected:          "cmd --pass ?",
			obfuscatedIndices: "11:24",
		},
		{
			command:           "cmd --pass=$(md5 file1 file2 file3)",
			expected:          "cmd --pass=?",
			obfuscatedIndices: "11:24",
		},
		{
			command:           "cmd $(md5 file1 file2 file3) --pass hello",
			expected:          "cmd $(md5 ? ? ?) --pass ?",
			obfuscatedIndices: "10:5 12:5 14:5 24:5",
		},
		{
			command:           "ENV=XXX cmd $(echo 'hello') --pass=secret",
			expected:          "ENV=? cmd $(echo 'hello') --pass=?",
			obfuscatedIndices: "4:3 33:6",
		},
		{
			command:           "cmd $(echo $(md5 'file.txt'))",
			expected:          "cmd $(echo $(md5 ?))",
			obfuscatedIndices: "17:10",
		},
		{
			command:           "cmd $(md5 file1 file2 file3) hello",
			expected:          "cmd $(md5 ? ? ?) hello",
			obfuscatedIndices: "10:5 12:5 14:5",
		},
		{
			command:           "cmd $(md5 $(echo 'file.txt'))",
			expected:          "cmd $(md5 ?)",
			obfuscatedIndices: "10:18",
		},
		{
			command:           "cmd $(echo $(echo $(md5 'file.txt')))",
			expected:          "cmd $(echo $(echo $(md5 ?)))",
			obfuscatedIndices: "24:10",
		},
		{
			command:           "echo hello $(",
			expected:          "echo hello $(",
			obfuscatedIndices: "",
		},
		{
			command:           "echo hello $(md5 file",
			expected:          "echo hello $(md5 ?",
			obfuscatedIndices: "17:4",
		},
		{
			command:           "$(echo `md5 file`) hello",
			expected:          "$(echo `md5 ?`) hello",
			obfuscatedIndices: "12:4",
		},
		{
			command:           "cmd \"$(echo file1 $(md5 file2) file3)\" hello",
			expected:          "cmd \"$(echo file1 $(md5 ?) file3)\" hello",
			obfuscatedIndices: "24:5",
		},
		{
			command:           "cmd \"$(md5 file1 $(md5 file2) file3)\" hello",
			expected:          "cmd \"$(md5 ? ? ?)\" hello",
			obfuscatedIndices: "11:5 13:12 15:5",
		},
		{
			command:           "cmd \"$(echo file1 `$(md5 file2)` `echo file3`)\" hello",
			expected:          "cmd \"$(echo file1 `$(md5 ?)` `echo file3`)\" hello",
			obfuscatedIndices: "25:5",
		},
		{
			command:           "cmd \"$(echo file1 `$(echo $(md5 file2))` `echo $(md5) file3`)\" hello '$(md5 file4)'",
			expected:          "cmd \"$(echo file1 `$(echo $(md5 ?))` `echo $(md5) file3`)\" hello '$(md5 file4)'",
			obfuscatedIndices: "32:5",
		},

		{
			command:           "ENV=${ENV:-production} ./script.sh --password=pony",
			expected:          "ENV=? ./script.sh --password=?",
			obfuscatedIndices: "4:18 29:4",
		},
		{
			command:           "echo $((2+3))",
			expected:          "echo $((2+3))",
			obfuscatedIndices: "",
		},
		{
			command:           "cmd $((2+3)) --pass=pony",
			expected:          "cmd $((2+3)) --pass=?",
			obfuscatedIndices: "20:4",
		},
		{
			command:           "cmd --pass `echo $(md5 file1)` `cmd --password $(md5 file2) --pass=\"$(md5 file3)\" --token $(md5 file4)` `md5 $(md5 file5) --pass` `cmd $(echo $(md5 file6 --pass)) --pass`",
			expected:          "cmd --pass ? `cmd --password ? --pass=? --token ?` `md5 ? ?` `cmd $(echo $(md5 ? ?)) --pass`",
			obfuscatedIndices: "11:19 29:12 38:14 48:12 56:12 58:6 79:5 81:6",
		},
		{
			command:           "cmd --pass=$(md5 file1) --password=`md5 file2` --pass $(md5 file3) --password=$(md5 file4) --pass=`md5 file5`",
			expected:          "cmd --pass=? --password=? --pass ? --password=? --pass=?",
			obfuscatedIndices: "11:12 24:11 33:12 46:12 55:11",
		},

		// Tests without any obfuscation
		{
			command:           "cmd --pass=; cmd2 --pass=",
			expected:          "cmd --pass=; cmd2 --pass=",
			obfuscatedIndices: "",
		},
		{
			command:           "cmd --pass=",
			expected:          "cmd --pass=",
			obfuscatedIndices: "",
		},
		{
			command:           "cmd --pass; cmd2 --pass",
			expected:          "cmd --pass; cmd2 --pass",
			obfuscatedIndices: "",
		},
		{
			command:           "cmd; cmd; LD_PRELOAD=XXX cmd2 --pass",
			expected:          "cmd; cmd; LD_PRELOAD=XXX cmd2 --pass",
			obfuscatedIndices: "",
		},
		{
			command:           "cmd --token; cmd --pass= ; LD_PRELOAD=$token cmd2 hello world",
			expected:          "cmd --token; cmd --pass= ; LD_PRELOAD=$token cmd2 hello world",
			obfuscatedIndices: "",
		},
		{
			command:           "cmd hello world; bin --pass; md5; md5 > txt.txt; md5",
			expected:          "cmd hello world; bin --pass; md5; md5 > txt.txt; md5",
			obfuscatedIndices: "",
		},
		{
			command:           "md5sum file",
			expected:          "md5sum file",
			obfuscatedIndices: "",
		},
	}

	assert := assert.New(t)
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			obfuscatedCmd, obfuscatedIndices := NewObfuscator(Config{}).ObfuscateShellCommand(tt.command)
			assert.Equal(tt.expected, obfuscatedCmd)
			assert.NotNil(obfuscatedIndices)
			assert.Equal(tt.obfuscatedIndices, obfuscatedIndices)
		})
	}
}

func TestCommandObfuscation(t *testing.T) {
	tests := []cmdTestCase{
		{
			command:           "[\"cmd\",\"--pass\",\"abc\",\"--token=def\"]",
			expected:          "[\"cmd\",\"--pass\",\"?\",\"--token=?\"]",
			obfuscatedIndices: "2 3-8",
		},
		{
			command:           "[\"cmd\",\"--pass\"]",
			expected:          "[\"cmd\",\"--pass\"]",
			obfuscatedIndices: "",
		},
		{
			command:           "[\"md5\",\"--password=pony\"]",
			expected:          "[\"md5\",\"?\"]",
			obfuscatedIndices: "1",
		},
		{
			command:           "[\"md5\",\"--pass=pony\"]",
			expected:          "[\"md5\",\"?\"]",
			obfuscatedIndices: "1",
		},
		{
			command:           "[\"md5\",\"--pass=\"]",
			expected:          "[\"md5\",\"?\"]",
			obfuscatedIndices: "1",
		},
		{
			command:           "[\"md5\",\"--pass\"]",
			expected:          "[\"md5\",\"?\"]",
			obfuscatedIndices: "1",
		},
		{
			command:           "[\"md5\",\"--pass\",\"pony\",\"hash\"]",
			expected:          "[\"md5\",\"?\",\"?\",\"?\"]",
			obfuscatedIndices: "1 2 3",
		},
		{
			command:           "[\"md5\",\"--pass=pony\",\"--pass\",\"pony\"]",
			expected:          "[\"md5\",\"?\",\"?\",\"?\"]",
			obfuscatedIndices: "1 2 3",
		},
		{
			command:           "[\"md5\",\"-s\",\"pony\"]",
			expected:          "[\"md5\",\"?\",\"?\"]",
			obfuscatedIndices: "1 2",
		},
		{
			command:           "[\"md5sum\",\"file\"]",
			expected:          "[\"md5sum\",\"file\"]",
			obfuscatedIndices: "",
		},

		// Shell commands
		{
			command:           "[\"bash\",\"-c\",\"md5 --password=pony\"]",
			expected:          "[\"bash\",\"-c\",\"md5 ?\"]",
			obfuscatedIndices: "2-4:15",
		},
		{
			command:           "[\"bash\",\"-c\",\"md5\"]",
			expected:          "[\"bash\",\"-c\",\"md5\"]",
			obfuscatedIndices: "",
		},
		{
			command:           "[\"bash\",\"-c\",\"md5 --password=\"]",
			expected:          "[\"bash\",\"-c\",\"md5 ?\"]",
			obfuscatedIndices: "2-4:11",
		},
		{
			command:           "[\"bash\",\"-c\",\"cat passwords.txt other | while read line; do; md5 -s $line; done\"]",
			expected:          "[\"bash\",\"-c\",\"cat passwords.txt other | while read line; do; md5 ? ?; done\"]",
			obfuscatedIndices: "2-51:2 2-53:5",
		},
		{
			command:           "[\"bash\",\"--pass=pony\",\"-c\",\"cat passwords.txt other | while read line; do; md5 -s $line; done\"]",
			expected:          "[\"bash\",\"--pass=?\",\"-c\",\"cat passwords.txt other | while read line; do; md5 ? ?; done\"]",
			obfuscatedIndices: "3-51:2 3-53:5 1-7",
		},
		{
			command:           "[\"bash\",\"-c\",\"cat passwords.txt other | while read line; do; md5 -s $line; done\",\"--pass=pony\"]",
			expected:          "[\"bash\",\"-c\",\"cat passwords.txt other | while read line; do; md5 ? ?; done\",\"--pass=?\"]",
			obfuscatedIndices: "2-51:2 2-53:5 3-7",
		},
		{
			command:           "[\"sh\",\"-c\",\"ENV=XXX LD_PRELOAD=YYY ls\"]",
			expected:          "[\"sh\",\"-c\",\"ENV=? LD_PRELOAD=YYY ls\"]",
			obfuscatedIndices: "2-4:3",
		},
	}

	assert := assert.New(t)
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			obfuscatedCmd, obfuscatedIndices, err := NewObfuscator(Config{}).ObfuscateExecCommand(tt.command)
			assert.NoError(err)
			assert.Equal(tt.expected, obfuscatedCmd)
			assert.NotNil(obfuscatedIndices)
			assert.Equal(tt.obfuscatedIndices, obfuscatedIndices)
		})
	}
}
