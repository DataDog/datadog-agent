package embed

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util"

	log "github.com/cihub/seelog"
	"github.com/kardianos/osext"
	"gopkg.in/yaml.v2"
)

type apmCheckConf struct {
	BinPath  string `yaml:"bin_path,omitempty"`
	ConfPath string `yaml:"conf_path,omitempty"`
}

// APMCheck keeps track of the running command
type APMCheck struct {
	cmd *exec.Cmd
}

func (c *APMCheck) String() string {
	return "APM Agent"
}

// Run executes the check
func (c *APMCheck) Run() error {
	// forward the standard output to the Agent logger
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stdout)
		for in.Scan() {
			log.Info(in.Text())
		}
	}()

	// forward the standard error to the Agent logger
	stderr, err := c.cmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		in := bufio.NewScanner(stderr)
		for in.Scan() {
			log.Error(in.Text())
		}
	}()

	if err = c.cmd.Start(); err != nil {
		return err
	}

	return c.cmd.Wait()
}

// Configure the APMCheck
func (c *APMCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	var checkConf apmCheckConf
	if err := yaml.Unmarshal(data, &checkConf); err != nil {
		return err
	}

	binPath := ""
	if checkConf.BinPath != "" {
		if _, err := os.Stat(checkConf.BinPath); err == nil {
			binPath = checkConf.BinPath
		} else {
			log.Infof("Can't access apm binary at %s, falling back to default", checkConf.BinPath)
		}
	}
	if binPath == "" {
		defaultBinPath, err := getAPMAgentDefaultBinPath()
		if err != nil {
			return err
		}
		binPath = defaultBinPath
	}

	// let the trace-agent use its own default config file if we haven't explicitly configured one
	ddConfigOption := ""
	if checkConf.ConfPath != "" {
		ddConfigOption = fmt.Sprintf("-ddconfig=%s", checkConf.ConfPath)
	}

	c.cmd = exec.Command(binPath, ddConfigOption)

	env := os.Environ()
	env = append(env, fmt.Sprintf("DD_HOSTNAME=%s", getHostname()))
	c.cmd.Env = env

	return nil
}

// InitSender initializes a sender but we don't need any
func (c *APMCheck) InitSender() {}

// Interval returns the scheduling time for the check, this will be scheduled only once
// since `Run` won't return, thus implementing a long running check.
func (c *APMCheck) Interval() time.Duration {
	return 0
}

// ID returns the name of the check since there should be only one instance running
func (c *APMCheck) ID() check.ID {
	return "APM_AGENT"
}

// Stop sends a termination signal to the APM process
func (c *APMCheck) Stop() {
	err := c.cmd.Process.Signal(os.Kill)
	if err != nil {
		log.Errorf("unable to stop APM check: %s", err)
	}
}

func init() {
	factory := func() check.Check {
		return &APMCheck{}
	}
	core.RegisterCheck("apm", factory)
}

func getHostname() string {
	hname, found := util.Cache.Get(path.Join(util.AgentCachePrefix, "hostname"))
	if found {
		return hname.(string)
	}
	return ""
}

func getAPMAgentDefaultBinPath() (string, error) {
	here, _ := osext.ExecutableFolder()
	binPath := path.Join(here, "..", "..", "embedded", "bin", "trace-agent")
	if _, err := os.Stat(binPath); err == nil {
		return binPath, nil
	}
	return "", fmt.Errorf("Can't access apm binary at %s", binPath)
}
