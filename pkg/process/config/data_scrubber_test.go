package config

import (
	"flag"
	"testing"
	"time"

	"github.com/DataDog/gopsutil/process"
	"github.com/stretchr/testify/assert"
)

func setupDataScrubber(t *testing.T) *DataScrubber {
	customSensitiveWords := []string{
		"consul_token",
		"dd_password",
		"blocked_from_yaml",
		"config",
		"pid",
	}

	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(customSensitiveWords)

	assert.Equal(t, true, scrubber.Enabled)
	assert.Equal(t, len(defaultSensitiveWords)+len(customSensitiveWords), len(scrubber.SensitivePatterns))

	return scrubber
}

func setupDataScrubberWildCard(t *testing.T) *DataScrubber {
	wildcards := []string{
		"*befpass",
		"afterpass*",
		"*both*",
		"mi*le",
		"*pass*d*",
		"*path*",
	}

	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(wildcards)

	assert.Equal(t, true, scrubber.Enabled)
	assert.Equal(t, len(defaultSensitiveWords)+len(wildcards), len(scrubber.SensitivePatterns))

	return scrubber
}

type testCase struct {
	cmdline       []string
	parsedCmdline []string
}

type testProcess struct {
	process.FilledProcess
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
		{[]string{"python ~/test/run.py --password=1234 -password 1234 -open_password=admin -consul_token 2345 -blocked_from_yaml=1234 &"},
			[]string{"python", "~/test/run.py", "--password=********", "-password", "********", "-open_password=admin", "-consul_token", "********", "-blocked_from_yaml=********", "&"}},
		{[]string{"agent", "-PASSWORD", "1234"}, []string{"agent", "-PASSWORD", "********"}},
		{[]string{"agent", "--PASSword", "1234"}, []string{"agent", "--PASSword", "********"}},
		{[]string{"agent", "--PaSsWoRd=1234"}, []string{"agent", "--PaSsWoRd=********"}},
		{[]string{"java -password      1234"}, []string{"java", "-password", "", "", "", "", "", "********"}},
		{[]string{"process-agent --config=datadog.yaml --pid=process-agent.pid"}, []string{"process-agent", "--config=********", "--pid=********"}},
		{[]string{"1-password --config=12345"}, []string{"1-password", "--config=********"}},
		{[]string{"java kafka password 1234"}, []string{"java", "kafka", "password", "********"}},
		{[]string{"agent", "password:1234"}, []string{"agent", "password:********"}},
		{[]string{"agent password:1234"}, []string{"agent", "password:********"}},
	}
}

func setupInsensitiveCmdlines() []testCase {
	return []testCase{
		{[]string{"spidly", "--debug_port=2043"}, []string{"spidly", "--debug_port=2043"}},
		{[]string{"agent", "start", "-p", "config.cfg"}, []string{"agent", "start", "-p", "config.cfg"}},
		{[]string{"p1", "--openpassword=admin"}, []string{"p1", "--openpassword=admin"}},
		{[]string{"p1", "-openpassword", "admin"}, []string{"p1", "-openpassword", "admin"}},
		{[]string{"java -openpassword 1234"}, []string{"java -openpassword 1234"}},
		{[]string{"java -open_password 1234"}, []string{"java -open_password 1234"}},
		{[]string{"java -passwordOpen 1234"}, []string{"java -passwordOpen 1234"}},
		{[]string{"java -password_open 1234"}, []string{"java -password_open 1234"}},
		{[]string{"java -password1 1234"}, []string{"java -password1 1234"}},
		{[]string{"java -password_1 1234"}, []string{"java -password_1 1234"}},
		{[]string{"java -1password 1234"}, []string{"java -1password 1234"}},
		{[]string{"java -1_password 1234"}, []string{"java -1_password 1234"}},
		{[]string{"agent", "1_password:1234"}, []string{"agent", "1_password:1234"}},
		{[]string{"agent 1_password:1234"}, []string{"agent 1_password:1234"}},
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

func setupTestProcesses() (fps []testProcess, sensible int) {
	cases := setupSensitiveCmdlines()
	sensible = len(cases)
	cases = append(cases, setupInsensitiveCmdlines()...)

	fps = make([]testProcess, 0, len(cases))
	for i, c := range cases {
		fps = append(fps, testProcess{
			process.FilledProcess{
				Pid:        int32(i),
				CreateTime: time.Now().Unix(),
				Cmdline:    c.cmdline,
			},
			c.parsedCmdline,
		})
	}

	return fps, sensible
}

func setupTestProcessesForBench() []testProcess {
	cases := setupSensitiveCmdlines()
	cases = append(cases, setupInsensitiveCmdlines()...)

	nbProcesses := 1200
	fps := make([]testProcess, 0, len(cases))
	for i := 0; i < nbProcesses; i++ {
		fps = append(fps, testProcess{
			process.FilledProcess{
				Pid:        int32(i),
				CreateTime: time.Now().Unix(),
				Cmdline:    cases[i%len(cases)].cmdline,
			},
			cases[i%len(cases)].parsedCmdline,
		})
	}

	return fps
}

func TestUncompilableWord(t *testing.T) {
	customSensitiveWords := []string{
		"consul_token",
		"dd_password",
		"(an_error",
		")a*",
		"[forbidden]",
		"]a*",
		"blocked_from_yaml",
		"*bef",
		"**bef",
		"after*",
		"after**",
		"*both*",
		"**both**",
		"mi*le",
		"mi**le",
		"*",
		"**",
		"*pass*d*",
	}

	validCustomSenstiveWords := []string{
		"consul_token",
		"dd_password",
		"blocked_from_yaml",
	}

	validWildCards := []string{
		"*bef",
		"after*",
		"*both*",
		"mi*le",
		"*pass*d*",
	}

	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(customSensitiveWords)

	assert.Equal(t, true, scrubber.Enabled)
	assert.Equal(t, len(defaultSensitiveWords)+len(validCustomSenstiveWords)+len(validWildCards), len(scrubber.SensitivePatterns))

	cases := []struct {
		cmdline       []string
		parsedCmdline []string
	}{
		{[]string{"process --consul_token=1234"}, []string{"process", "--consul_token=********"}},
		{[]string{"process --dd_password=1234"}, []string{"process", "--dd_password=********"}},
		{[]string{"process --blocked_from_yaml=1234"}, []string{"process", "--blocked_from_yaml=********"}},

		{[]string{"process --onebef=1234"}, []string{"process", "--onebef=********"}},
		{[]string{"process --afterone=1234"}, []string{"process", "--afterone=********"}},
		{[]string{"process --oneboth1=1234"}, []string{"process", "--oneboth1=********"}},
		{[]string{"process --middle=1234"}, []string{"process", "--middle=********"}},
		{[]string{"process --twopasswords=1234,5678"}, []string{"process", "--twopasswords=********"}},
	}

	for i := range cases {
		cases[i].cmdline, _ = scrubber.scrubCommand(cases[i].cmdline)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}
}

func TestBlacklistedArgs(t *testing.T) {
	cases := setupSensitiveCmdlines()
	scrubber := setupDataScrubber(t)

	for i := range cases {
		cases[i].cmdline, _ = scrubber.scrubCommand(cases[i].cmdline)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}
}

func TestBlacklistedArgsWhenDisabled(t *testing.T) {
	cases := []struct {
		cmdline       []string
		parsedCmdline []string
	}{
		{[]string{"agent", "-password", "1234"}, []string{"agent", "-password", "1234"}},
		{[]string{"agent", "--password", "1234"}, []string{"agent", "--password", "1234"}},
		{[]string{"agent", "-password=1234"}, []string{"agent", "-password=1234"}},
		{[]string{"agent", "--password=1234"}, []string{"agent", "--password=1234"}},
		{[]string{"fitz", "-consul_token=1234567890"}, []string{"fitz", "-consul_token=1234567890"}},
		{[]string{"fitz", "--consul_token=1234567890"}, []string{"fitz", "--consul_token=1234567890"}},
		{[]string{"fitz", "-consul_token", "1234567890"}, []string{"fitz", "-consul_token", "1234567890"}},
		{[]string{"fitz", "--consul_token", "1234567890"}, []string{"fitz", "--consul_token", "1234567890"}},
		{[]string{"python ~/test/run.py --password=1234 -password 1234 -open_password=admin -consul_token 2345 -blocked_from_yaml=1234 &"},
			[]string{"python ~/test/run.py --password=1234 -password 1234 -open_password=admin -consul_token 2345 -blocked_from_yaml=1234 &"}},
		{[]string{"agent", "-PASSWORD", "1234"}, []string{"agent", "-PASSWORD", "1234"}},
		{[]string{"agent", "--PASSword", "1234"}, []string{"agent", "--PASSword", "1234"}},
		{[]string{"agent", "--PaSsWoRd=1234"}, []string{"agent", "--PaSsWoRd=1234"}},
		{[]string{"java -password      1234"}, []string{"java -password      1234"}},
		{[]string{"agent", "password:1234"}, []string{"agent", "password:1234"}},
		{[]string{"agent password:1234"}, []string{"agent password:1234"}},
	}

	scrubber := setupDataScrubber(t)
	scrubber.Enabled = false

	for i := range cases {
		fp := &process.FilledProcess{Cmdline: cases[i].cmdline}
		cases[i].cmdline = scrubber.ScrubProcessCommand(fp)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}
}

func TestScrubberStrippingAllArgument(t *testing.T) {
	cases := []struct {
		cmdline       []string
		parsedCmdline []string
	}{
		{[]string{"agent", "-password", "1234"}, []string{"agent"}},
		{[]string{"agent", "--password", "1234"}, []string{"agent"}},
		{[]string{"agent", "-password=1234"}, []string{"agent"}},
		{[]string{"agent", "--password=1234"}, []string{"agent"}},
		{[]string{"fitz", "-consul_token=1234567890"}, []string{"fitz"}},
		{[]string{"fitz", "--consul_token=1234567890"}, []string{"fitz"}},
		{[]string{"fitz", "-consul_token", "1234567890"}, []string{"fitz"}},
		{[]string{"fitz", "--consul_token", "1234567890"}, []string{"fitz"}},
		{
			[]string{"python ~/test/run.py --password=1234 -password 1234 -open_password=admin -consul_token 2345 -blocked_from_yaml=1234 &"},
			[]string{"python"},
		},
		{[]string{"agent", "-PASSWORD", "1234"}, []string{"agent"}},
		{[]string{"agent", "--PASSword", "1234"}, []string{"agent"}},
		{[]string{"agent", "--PaSsWoRd=1234"}, []string{"agent"}},
		{[]string{"java -password      1234"}, []string{"java"}},
		{[]string{"agent", "password:1234"}, []string{"agent"}},
		{[]string{"agent password:1234"}, []string{"agent"}},
	}

	scrubber := setupDataScrubber(t)
	scrubber.StripAllArguments = true

	for i := range cases {
		fp := &process.FilledProcess{Cmdline: cases[i].cmdline}
		cases[i].cmdline = scrubber.ScrubProcessCommand(fp)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}
}

func TestNoBlacklistedArgs(t *testing.T) {
	cases := setupInsensitiveCmdlines()
	scrubber := setupDataScrubber(t)

	for i := range cases {
		cases[i].cmdline, _ = scrubber.scrubCommand(cases[i].cmdline)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}
}

func TestMatchWildCards(t *testing.T) {
	cases := setupCmdlinesWithWildCards()
	scrubber := setupDataScrubberWildCard(t)

	for i := range cases {
		cases[i].cmdline, _ = scrubber.scrubCommand(cases[i].cmdline)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}
}

func TestScrubWithCache(t *testing.T) {
	testProcs, sensible := setupTestProcesses()
	scrubber := setupDataScrubber(t)

	// During the cache lifespan, all the processes scrubbed cmdline must live in the cache
	for i := 0; i < int(scrubber.cacheMaxCycles); i++ {
		for _, p := range testProcs {
			scrubbed := scrubber.ScrubProcessCommand(&p.FilledProcess)
			assert.Equal(t, p.parsedCmdline, scrubbed)
		}
		assert.Equal(t, len(testProcs), len(scrubber.seenProcess))
		assert.Equal(t, sensible, len(scrubber.scrubbedCmdlines))
		scrubber.IncrementCacheAge()
	}

	// When we reach the cache ttl, it should be empty
	assert.Equal(t, 0, len(scrubber.seenProcess))
	assert.Equal(t, 0, len(scrubber.scrubbedCmdlines))

	// Scrubbing the same processes should put them again on cache
	for _, p := range testProcs {
		scrubbed := scrubber.ScrubProcessCommand(&p.FilledProcess)
		assert.Equal(t, p.parsedCmdline, scrubbed)
	}
	assert.Equal(t, len(testProcs), len(scrubber.seenProcess))
	assert.Equal(t, sensible, len(scrubber.scrubbedCmdlines))
}

func BenchmarkRegexMatching1(b *testing.B)    { benchmarkRegexMatching(1, b) }
func BenchmarkRegexMatching10(b *testing.B)   { benchmarkRegexMatching(10, b) }
func BenchmarkRegexMatching100(b *testing.B)  { benchmarkRegexMatching(100, b) }
func BenchmarkRegexMatching1000(b *testing.B) { benchmarkRegexMatching(1000, b) }

var avoidOptimization []string

func benchmarkRegexMatching(nbProcesses int, b *testing.B) {
	runningProcesses := make([][]string, nbProcesses)
	foolCmdline := []string{"python ~/test/run.py --password=1234 -password 1234 -password=admin -secret 2345 -credentials=1234 -api_key 2808 &"}

	customSensitiveWords := []string{
		"*consul_token",
		"*dd_password",
		"*blocked_from_yaml",
	}
	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(customSensitiveWords)

	for i := 0; i < nbProcesses; i++ {
		runningProcesses = append(runningProcesses, foolCmdline)
	}

	var r []string
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, p := range runningProcesses {
			r, _ = scrubber.scrubCommand(p)
		}
	}

	avoidOptimization = r
}

var useCache = flag.Bool("cache", true, "enable/disable the use of cache on BenchmarkCache")

func BenchmarkCache(b *testing.B) {
	if *useCache {
		benchmarkWithCache(b)
	} else {
		benchmarkWithoutCache(b)
	}
}

func benchmarkWithCache(b *testing.B) {
	customSensitiveWords := []string{
		"*consul_token",
		"*dd_password",
		"*blocked_from_yaml",
	}
	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(customSensitiveWords)
	fps := setupTestProcessesForBench()

	var r []string
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := 0; i < len(fps); i++ {
			r = scrubber.ScrubProcessCommand(&fps[i].FilledProcess)
		}
	}
	avoidOptimization = r
}

func benchmarkWithoutCache(b *testing.B) {
	customSensitiveWords := []string{
		"*consul_token",
		"*dd_password",
		"*blocked_from_yaml",
	}
	scrubber := NewDefaultDataScrubber()
	scrubber.AddCustomSensitiveWords(customSensitiveWords)
	fps := setupTestProcessesForBench()

	var r []string
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := 0; i < len(fps); i++ {
			r, _ = scrubber.scrubCommand(fps[i].Cmdline)
		}
	}
	avoidOptimization = r
}
