// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build orchestrator

package orchestrator

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/process/config"
)

func BenchmarkNoRegexMatching1(b *testing.B)        { benchmarkMatching(1, b) }
func BenchmarkNoRegexMatching10(b *testing.B)       { benchmarkMatching(10, b) }
func BenchmarkNoRegexMatching100(b *testing.B)      { benchmarkMatching(100, b) }
func BenchmarkNoRegexMatching1000(b *testing.B)     { benchmarkMatching(5000, b) }
func BenchmarkRegexMatchingCustom1000(b *testing.B) { benchmarkMatchingCustomRegex(1000, b) }

// https://dave.cheney.net/2013/06/30/how-to-write-benchmarks-in-go
// store the result to a package level variable
// so the compiler cannot eliminate the Benchmark itself.
//goland:noinspection ALL
var avoidOptimization bool

func benchmarkMatching(nbContainers int, b *testing.B) {
	containersBenchmarks := make([]v1.Container, nbContainers)
	containersToBenchmark := make([]v1.Container, nbContainers)
	var changed bool
	cfg := config.NewDefaultAgentConfig(true)
	scrubber := NewDefaultDataScrubber()
	for _, testCase := range getScrubCases() {
		containersToBenchmark = append(containersToBenchmark, testCase.input)
	}
	for i := 0; i < nbContainers; i++ {
		containersBenchmarks = append(containersBenchmarks, containersToBenchmark...)
	}
	b.ResetTimer()

	b.Run(fmt.Sprintf("simplified"), func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			for _, c := range containersBenchmarks {
				changed = ScrubContainer(&c, scrubber)
			}
		}
	})

	b.Run(fmt.Sprintf("default"), func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			for _, c := range containersBenchmarks {
				changed = ScrubContainerOld(&c, cfg)
			}
		}
	})

	avoidOptimization = changed
}

func benchmarkMatchingCustomRegex(nbContainers int, b *testing.B) {
	var changed bool

	var containersBenchmarks []v1.Container
	var containersToBenchmark []v1.Container

	customRegs := []string{"pwd*", "*test"}
	cfg := config.NewDefaultAgentConfig(true)
	cfg.Scrubber.AddCustomSensitiveWords(customRegs)
	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveRegex(customRegs)

	for _, testCase := range getScrubCases() {
		containersToBenchmark = append(containersToBenchmark, testCase.input)
	}
	for i := 0; i < nbContainers; i++ {
		containersBenchmarks = append(containersBenchmarks, containersToBenchmark...)
	}
	b.ResetTimer()
	b.Run(fmt.Sprintf("simplified"), func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			for _, c := range containersBenchmarks {
				changed = ScrubContainer(&c, scrubber)
			}
		}
	})

	b.Run(fmt.Sprintf("default"), func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			for _, c := range containersBenchmarks {
				changed = ScrubContainerOld(&c, cfg)
			}
		}
	})

	avoidOptimization = changed
}

func TestMatchSimpleCommand(t *testing.T) {
	cases := setupSensitiveCmdlines()
	customSensitiveWords := []string{
		"consul_token",
		"dd_password",
		"blocked_from_yaml",
		"config",
		"pid",
	}

	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(customSensitiveWords)

	for i := range cases {
		cases[i].cmdline, _ = scrubber.ScrubSimpleCommand(cases[i].cmdline)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}
}

func TestMatchNoMatchCommand(t *testing.T) {
	cases := setupInsensitiveCmdLines()

	scrubber := NewDefaultDataScrubber()

	for i := range cases {
		cases[i].cmdline, _ = scrubber.ScrubSimpleCommand(cases[i].cmdline)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}
}

func TestMatchSimpleCommandScrubRegex(t *testing.T) {
	cases := setupCmdlinesWithWildCards()
	customSensitiveWords := []string{"passwd"}

	wildcards := []string{
		"*path*",
		"*both*",
		"*befpass",
		"afterpass*",
		"mi*le",
	}

	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(customSensitiveWords)
	scrubber.AddCustomSensitiveRegex(wildcards)

	for i := range cases {
		cases[i].cmdline, _ = scrubber.ScrubSimpleCommand(cases[i].cmdline)
		println(cases[i].cmdline)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}
}

func BenchmarkCommandMatching1(b *testing.B)    { benchmarkCommandMatching(1, b) }
func BenchmarkCommandMatching10(b *testing.B)   { benchmarkCommandMatching(10, b) }
func BenchmarkCommandMatching100(b *testing.B)  { benchmarkCommandMatching(100, b) }
func BenchmarkCommandMatching1000(b *testing.B) { benchmarkCommandMatching(1000, b) }

func benchmarkCommandMatching(nbCommands int, b *testing.B) {
	runningProcesses := make([][]string, nbCommands)
	var c bool
	foolCmdline := []string{"python ~/test/run.py --dd_password=1234 -password 1234 -password=admin -secret 2345 -credentials=1234 -api_key 2808 &"}

	customSensitiveRegex := []string{
		"*consul_token",
		"*dd_password",
		"*blocked_from_yaml",
	}
	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveRegex(customSensitiveRegex)

	cfgScrubber := config.NewDefaultDataScrubber()
	cfgScrubber.AddCustomSensitiveWords(customSensitiveRegex)

	for i := 0; i < nbCommands; i++ {
		runningProcesses[i] = foolCmdline
	}

	b.ResetTimer()

	b.Run(fmt.Sprintf("simplified"), func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			for _, p := range runningProcesses {
				_, c = scrubber.ScrubSimpleCommand(p)
			}
		}
	})

	b.Run(fmt.Sprintf("default"), func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			for _, p := range runningProcesses {
				_, c = cfgScrubber.ScrubCommand(p)
			}
		}
	})

	avoidOptimization = c
}

type testCase struct {
	cmdline       []string
	parsedCmdline []string
}

func setupSensitiveCmdlines() []testCase {
	return []testCase{
		{[]string{"agent", "-password", "1234"}, []string{"agent", "-password", "********"}},
		{[]string{"agent", "--password", "1234"}, []string{"agent", "--password", "********"}},
		{[]string{"agent", "-password=1234"}, []string{"agent", "-password=********"}},
		{[]string{"agent", "--password=1234"}, []string{"agent", "--password=********"}},
		{[]string{"fitz", "-consul_token=1234567890"}, []string{"fitz", "-consul_token=********"}},
		{[]string{"fitz", "--consul_token=1234567890"}, []string{"fitz", "--consul_token=********"}},
		{[]string{"fitz", "-consul_token", "1234567890"}, []string{"fitz", "-consul_token", "********"}},
		{[]string{"fitz", "--consul_token", "1234567890"}, []string{"fitz", "--consul_token", "********"}},
		{[]string{"fitz", "-dd_password", "1234567890"}, []string{"fitz", "-dd_password", "********"}},
		{[]string{"fitz", "dd_password", "1234567890"}, []string{"fitz", "dd_password", "********"}},
		{[]string{"python ~/test/run.py --password=1234 -password 1234 -open_pword=admin -consul_token 2345 -blocked_from_yaml=1234 &"},
			[]string{"python", "~/test/run.py", "--password=********", "-password", "********", "-open_pword=admin", "-consul_token", "********", "-blocked_from_yaml=********", "&"}},
		{[]string{"agent", "-PASSWORD", "1234"}, []string{"agent", "-PASSWORD", "********"}},
		{[]string{"agent", "--PASSword", "1234"}, []string{"agent", "--PASSword", "********"}},
		{[]string{"agent", "--PaSsWoRd=1234"}, []string{"agent", "--PaSsWoRd=********"}},
		{[]string{"java -password      1234"}, []string{"java", "-password", "", "", "", "", "", "********"}},
		{[]string{"process-agent --config=datadog.yaml --pid=process-agent.pid"}, []string{"process-agent", "--config=********", "--pid=********"}},
		{[]string{"1-password --config=12345"}, []string{"1-password", "--config=********"}}, // not working
		{[]string{"java kafka password 1234"}, []string{"java", "kafka", "password", "********"}},
		{[]string{"agent", "password:1234"}, []string{"agent", "password:********"}},
		{[]string{"agent password:1234"}, []string{"agent", "password:********"}},
		{[]string{"p1", "--openpassword=admin"}, []string{"p1", "--openpassword=********"}},
		{[]string{"p1", "-openpassword", "admin"}, []string{"p1", "-openpassword", "********"}},
		{[]string{"java -openpassword 1234"}, []string{"java", "-openpassword", "********"}},
		{[]string{"java -open_password 1234"}, []string{"java", "-open_password", "********"}},
		{[]string{"java -passwordOpen 1234"}, []string{"java", "-passwordOpen", "********"}},
		{[]string{"java -password_open 1234"}, []string{"java", "-password_open", "********"}},
		{[]string{"java -password1 1234"}, []string{"java", "-password1", "********"}},
		{[]string{"java -password_1 1234"}, []string{"java", "-password_1", "********"}},
		{[]string{"java -1password 1234"}, []string{"java", "-1password", "********"}},
		{[]string{"java -1_password 1234"}, []string{"java", "-1_password", "********"}},
		{[]string{"agent", "1_password:1234"}, []string{"agent", "1_password:********"}},
		{[]string{"agent 1_password:1234"}, []string{"agent", "1_password:********"}},
	}
}

func setupCmdlinesWithWildCards() []testCase {
	return []testCase{
		{[]string{"spidly", "--befpass=2043", "onebefpass", "1234", "--befpassCustom=1234"},
			[]string{"spidly", "--befpass=********", "onebefpass", "********", "--befpassCustom=1234"}},
		{[]string{"spidly --befpass=2043 onebefpass 1234 --befpassCustom=1234"},
			[]string{"spidly", "--befpass=********", "onebefpass", "********", "--befpassCustom=1234"}},
		{[]string{"spidly   --befpass=2043   onebefpass   1234   --befpassCustom=1234"},
			[]string{"spidly", "", "", "--befpass=********", "", "", "onebefpass", "", "", "********", "", "", "--befpassCustom=1234"}},

		{[]string{"spidly", "--afterpass=2043", "afterpass_1", "1234", "--befafterpass_1=1234"},
			[]string{"spidly", "--afterpass=********", "afterpass_1", "********", "--befafterpass_1=1234"}},
		{[]string{"spidly --afterpass=2043 afterpass_1 1234 --befafterpass_1=1234"},
			[]string{"spidly", "--afterpass=********", "afterpass_1", "********", "--befafterpass_1=1234"}},
		{[]string{"spidly   --afterpass=2043   afterpass_1   1234   --befafterpass_1=1234"},
			[]string{"spidly", "", "", "--afterpass=********", "", "", "afterpass_1", "", "", "********", "", "", "--befafterpass_1=1234"}},

		{[]string{"spidly", "both", "1234", "-dd_both", "1234", "bothafter", "1234", "--dd_bothafter=1234"},
			[]string{"spidly", "both", "********", "-dd_both", "********", "bothafter", "********", "--dd_bothafter=********"}},
		{[]string{"spidly both 1234 -dd_both 1234 bothafter 1234 --dd_bothafter=1234"},
			[]string{"spidly", "both", "********", "-dd_both", "********", "bothafter", "********", "--dd_bothafter=********"}},
		{[]string{"spidly   both   1234   -dd_both   1234   bothafter   1234   --dd_bothafter=1234"},
			[]string{"spidly", "", "", "both", "", "", "********", "", "", "-dd_both", "", "", "********", "", "", "bothafter", "", "", "********", "", "", "--dd_bothafter=********"}},

		{[]string{"spidly", "middle", "1234", "-mile", "1234", "--mill=1234"},
			[]string{"spidly", "middle", "********", "-mile", "********", "--mill=1234"}},
		{[]string{"spidly middle 1234 -mile 1234 --mill=1234"},
			[]string{"spidly", "middle", "********", "-mile", "********", "--mill=1234"}},
		{[]string{"spidly   middle   1234   -mile   1234   --mill=1234"},
			[]string{"spidly", "", "", "middle", "", "", "********", "", "", "-mile", "", "", "********", "", "", "--mill=1234"}},

		{[]string{"spidly", "--passwd=1234", "password", "1234", "-mypassword", "1234", "--passwords=12345,123456", "--mypasswords=1234,123456"},
			[]string{"spidly", "--passwd=********", "password", "********", "-mypassword", "********", "--passwords=********", "--mypasswords=********"}},
		{[]string{"spidly --passwd=1234 password 1234 -mypassword 1234 --passwords=12345,123456 --mypasswords=1234,123456"},
			[]string{"spidly", "--passwd=********", "password", "********", "-mypassword", "********", "--passwords=********", "--mypasswords=********"}},
		{[]string{"spidly   --passwd=1234   password   1234   -mypassword   1234   --passwords=12345,123456   --mypasswords=1234,123456"},
			[]string{"spidly", "", "", "--passwd=********", "", "", "password", "", "", "********", "", "", "-mypassword", "", "", "********",
				"", "", "--passwords=********", "", "", "--mypasswords=********"}},

		{[]string{"run-middle password 12345"}, []string{"run-middle", "password", "********"}},
		{[]string{"generate-password -password 12345"}, []string{"generate-password", "-password", "********"}},
		{[]string{"generate-password --password=12345"}, []string{"generate-password", "--password=********"}},

		{[]string{"java /var/lib/datastax-agent/conf/address.yaml -Dopscenter.ssl.keyStorePassword=opscenter -Dagent-pidfile=/var/run/datastax-agent/datastax-agent.pid --anotherpassword=1234"},
			[]string{"java", "/var/lib/datastax-agent/conf/address.yaml", "-Dopscenter.ssl.keyStorePassword=********", "-Dagent-pidfile=/var/run/datastax-agent/datastax-agent.pid", "--anotherpassword=********"}},

		{[]string{"/usr/bin/java -Des.path.home=/usr/local/elasticsearch-1.7.6 -cp $ES_CLASSPATH:$ES_HOME/lib/*:$ES_HOME/lib/sigar/*:/usr/local/elasticsearch-1.7.6" +
			"/lib/elasticsearch-1.7.6.jar:/usr/local/elasticsearch-1.7.6/lib/*:/usr/local/elasticsearch-1.7.6/lib" +
			"/sigar/* org.elasticsearch.bootstrap.Elasticsearch"},
			[]string{"/usr/bin/java", "-Des.path.home=********", "-cp", "$ES_CLASSPATH:$ES_HOME/lib/*:$ES_HOME/lib/sigar/*:/usr/local/elasticsearch-1.7.6" +
				"/lib/elasticsearch-1.7.6.jar:/usr/local/elasticsearch-1.7.6/lib/*:/usr/local/elasticsearch-1.7.6/lib/sigar/*",
				"org.elasticsearch.bootstrap.Elasticsearch"}},

		{[]string{"process ‑XXpath:/secret/"}, []string{"process", "‑XXpath:********"}},
		{[]string{"process", "‑XXpath:/secret/"}, []string{"process", "‑XXpath:********"}},
	}
}

func setupInsensitiveCmdLines() []testCase {
	return []testCase{
		{[]string{"spidly", "--debug_port=2043"}, []string{"spidly", "--debug_port=2043"}},
		{[]string{"agent", "start", "-p", "config.cfg"}, []string{"agent", "start", "-p", "config.cfg"}},
		{[]string{"p1", "-user=admin"}, []string{"p1", "-user=admin"}},
		{[]string{"p1", "-user", "admin"}, []string{"p1", "-user", "admin"}},
		{[]string{"java -xMg 1234"}, []string{"java -xMg 1234"}},
		{[]string{"java -open_pword 1234"}, []string{"java -open_pword 1234"}},
		{[]string{"java -pwordOpen 1234"}, []string{"java -pwordOpen 1234"}},
		{[]string{"java -pword_open 1234"}, []string{"java -pword_open 1234"}},
		{[]string{"java -pword1 1234"}, []string{"java -pword1 1234"}},
		{[]string{"java -pword_1 1234"}, []string{"java -pword_1 1234"}},
		{[]string{"java -1pword 1234"}, []string{"java -1pword 1234"}},
		{[]string{"java -1_pword 1234"}, []string{"java -1_pword 1234"}},
		{[]string{"agent", "1_pword:1234"}, []string{"agent", "1_pword:1234"}},
		{[]string{"agent 1_pword:1234"}, []string{"agent 1_pword:1234"}},
	}
}

