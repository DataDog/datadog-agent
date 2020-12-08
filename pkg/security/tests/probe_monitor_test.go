package tests

import (
	"fmt"
	"github.com/cihub/seelog"
	"os"
	"path"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestProbeMonitor(t *testing.T) {
	var truncatedParents, truncatedSegment string
	for i := 0; i <= probe.MaxPathDepth; i++ {
		truncatedParents += "a/"
	}
	for i := 0; i <= probe.MaxSegmentLength+1; i++ {
		truncatedSegment += "a"
	}

	rule := &rules.RuleDefinition{
		ID:         "path_test",
		Expression: `open.filename =~ "*a/test-open" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}

	truncatedParentsFile, truncatedParentsFilePtr, err := test.Path(fmt.Sprintf("%stest-open", truncatedParents))
	if err != nil {
		t.Fatal(err)
	}

	truncatedSegmentFile, truncatedSegmentFilePtr, err := test.Path(fmt.Sprintf("%s/test-open", truncatedSegment))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ruleset_loaded", func(t *testing.T) {
		test.Close()
		test, err = newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
		if err != nil {
			t.Fatal(err)
		}
		defer test.Close()

		ruleEvent, err := test.GetProbeCustomEvent(1*time.Second, probe.RuleSetLoadedRuleID)
		if err != nil {
			t.Error(err)
		} else {
			if ruleEvent.RuleID != probe.RuleSetLoadedRuleID {
				t.Errorf("expected %s rule, got %s", probe.RuleSetLoadedRuleID, ruleEvent.RuleID)
			}
		}
	})

	t.Run("truncated_segment", func(t *testing.T) {
		if os.MkdirAll(path.Dir(truncatedSegmentFile), 0755) != nil {
			t.Fatal(err)
		}
		fd, _, errno := syscall.Syscall(syscall.SYS_OPEN, uintptr(truncatedSegmentFilePtr), syscall.O_CREAT, 0755)
		if errno != 0 {
			t.Fatal(error(errno))
		}
		defer os.Remove(truncatedSegmentFile)
		defer syscall.Close(int(fd))

		ruleEvent, err := test.GetProbeCustomEvent(3*time.Second, probe.ErrTruncatedSegment{}.Error())
		if err != nil {
			t.Error(err)
		} else {
			if ruleEvent.RuleID != probe.AbnormalPathRuleID {
				t.Errorf("expected %s rule, got %s", probe.AbnormalPathRuleID, ruleEvent.RuleID)
			}
		}
	})

	t.Run("truncated_parents", func(t *testing.T) {
		if os.MkdirAll(path.Dir(truncatedParentsFile), 0755) != nil {
			t.Fatal(err)
		}
		fd, _, errno := syscall.Syscall(syscall.SYS_OPEN, uintptr(truncatedParentsFilePtr), syscall.O_CREAT, 0755)
		if errno != 0 {
			t.Fatal(error(errno))
		}
		defer os.Remove(truncatedParentsFile)
		defer syscall.Close(int(fd))

		ruleEvent, err := test.GetProbeCustomEvent(3*time.Second, probe.ErrTruncatedParents{}.Error())
		if err != nil {
			t.Error(err)
		} else {
			if ruleEvent.RuleID != probe.AbnormalPathRuleID {
				t.Errorf("expected %s rule, got %s", probe.AbnormalPathRuleID, ruleEvent.RuleID)
			}
		}
	})

	t.Run("fork_bomb", func(t *testing.T) {
		if err := test.st.setLogLevel(seelog.DebugLvl); err != nil {
			t.Error(err)
		}

		executable := "/usr/bin/touch"
		if resolved, err := os.Readlink(executable); err == nil {
			executable = resolved
		} else {
			if os.IsNotExist(err) {
				executable = "/bin/touch"
			}
		}

		go func() {
			for i := int64(0); i < testMod.config.LoadControllerForkBombThreshold*12/10; i++ {
				args := []string{"touch", "/dev/null"}
				_, err := syscall.ForkExec(executable, args, nil)
				if err != nil {
					t.Error(err)
				}
			}
		}()

		ruleEvent, err := test.GetProbeCustomEvent(3*time.Second, probe.ForkBombRuleID)
		if err != nil {
			t.Error(err)
		} else {
			if ruleEvent.RuleID != probe.ForkBombRuleID {
				t.Errorf("expected %s rule, got %s", probe.ForkBombRuleID, ruleEvent.RuleID)
			}
		}

		time.Sleep(3 * time.Second)

		if err := test.st.setLogLevel(seelog.TraceLvl); err != nil {
			t.Error(err)
		}
	})
}
