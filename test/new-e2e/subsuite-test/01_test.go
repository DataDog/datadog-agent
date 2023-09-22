package subSuite

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
)

type vmFakeintakeSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func logsExampleStackDef(vmParams []ec2params.Option, agentParams ...agentparams.Option) *e2e.StackDefinition[e2e.FakeIntakeEnv] {
	return e2e.FakeIntakeStackDef(nil, agentparams.WithLogs())

}
func TestE2EVMFakeintakeSuite(t *testing.T) {
	e2e.Run(t, &vmFakeintakeSuite{}, logsExampleStackDef(nil), params.WithDevMode())
}

func (s *vmFakeintakeSuite) Test3LinuxTests() {
	s.T().Run("LinuxSubTest1", func(t *testing.T) {
		s.LinuxSubTest1()
	})

	time.Sleep(1 * time.Second)

	s.T().Run("LinuxSubTest2", func(t *testing.T) {
		s.LinuxSubTest2()
	})

	time.Sleep(1 * time.Second)

	s.T().Run("LinuxSubTest2", func(t *testing.T) {
		s.LinuxSubTest3()
	})
}
func (s *vmFakeintakeSuite) LinuxSubTest1() {
	s.Env().VM.Execute("sudo touch /var/log/meowtwo.log ")
	time.Sleep(1 * time.Second)
	ls := s.Env().VM.Execute("ls /var/log/meowtwo.log")
	s.T().Logf("%s", ls)
}

func (s *vmFakeintakeSuite) LinuxSubTest2() {
	s.Env().VM.Execute("sudo touch /var/log/meowth.log ")
	time.Sleep(1 * time.Second)
	ls := s.Env().VM.Execute("ls /var/log/meowth.log")
	s.T().Logf("%s", ls)
}

func (s *vmFakeintakeSuite) LinuxSubTest3() {
	s.Env().VM.Execute("sudo touch /var/log/meowtra.log ")
	time.Sleep(1 * time.Second)
	ls := s.Env().VM.Execute("ls /var/log/meowtra.log")
	s.T().Logf("%s", ls)
}
