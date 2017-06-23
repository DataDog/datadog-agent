// +build apm

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
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	log "github.com/cihub/seelog"
	"github.com/kardianos/osext"
)

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
	here, _ := osext.ExecutableFolder()
	dist := path.Join(here, "dist")
	bin := path.Join(here, "trace-agent")
	conf := path.Join(dist, "trace-agent.ini")

	c.cmd = exec.Command(bin, fmt.Sprintf("-ddconfig=%s", conf))

	env := os.Environ()
	env = append(env, "DD_APM_ENABLED=true")
	env = append(env, fmt.Sprintf("DD_API_KEY=%s", config.Datadog.GetString("api_key")))
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
