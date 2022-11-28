package rules

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Masterminds/semver/v3"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

var (
	ErrEmptyPath               = errors.New("policy file path is empty")
	ErrEmptyRuleID             = errors.New("rule ID is empty")
	ErrInvalidRuleTargetType   = errors.New("rule target type is invalid")
	ErrUnsupportedRuleAction   = errors.New("rule action is unsupported for now")
	ErrMissingPathOrName       = errors.New("either the target absolute path or the target basename is required")
	ErrMissingFunctionOrOffset = errors.New("either the function name or the offset is required")
)

func LoadPolicyFromFile(path string) (*Policy, error) {
	if len(path) == 0 {
		return nil, ErrEmptyPath
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	name := filepath.Base(path)

	return loadPolicy(f, name, path)
}

func sanitizePolicyDefinition(def PolicyDefinition, name string, source string) (*Policy, error) {
	policy := &Policy{
		Name:   name,
		Source: source,
	}

	version, err := semver.NewVersion(def.Version)
	if err != nil {
		return nil, err
	}
	policy.Version = version

	ruleIDs := make(map[string]*RuleDefinition)

	for index, ruleDef := range def.Rules {
		if len(ruleDef.ID) == 0 {
			return nil, ErrEmptyRuleID
		}

		if ruleDef.Disabled {
			logrus.Infof("rule %s (%d) is marked as disabled, ignoring it", ruleDef.ID, index)
			continue
		}

		if ruleWithSameID, ok := ruleIDs[ruleDef.ID]; ok {
			logrus.Infof("rule %d has the same id as rule %d (%s), ignoring it", ruleWithSameID.Index, index, ruleDef.ID)
			continue
		}

		if ruleDef.TargetType != "library" && ruleDef.TargetType != "binary" {
			return nil, ErrInvalidRuleTargetType
		}

		if len(ruleDef.TargetPath) == 0 && len(ruleDef.TargetName) == 0 {
			return nil, ErrMissingPathOrName
		}

		if len(ruleDef.TargetFunction) == 0 && len(ruleDef.TargetOffset) == 0 {
			return nil, ErrMissingFunctionOrOffset
		}

		if len(ruleDef.TargetOffset) > 0 {
			offset, err := strconv.ParseUint(ruleDef.TargetOffset, 0, 64)
			if err != nil {
				return nil, err
			}
			ruleDef.Offset = offset
		}

		if len(ruleDef.Action) > 0 {
			return nil, ErrUnsupportedRuleAction
		}

		ruleDef.Index = uint64(index)
		ruleIDs[ruleDef.ID] = ruleDef
	}

	policy.Rules = ruleIDs

	return policy, nil
}

func loadPolicy(reader io.Reader, name string, source string) (*Policy, error) {
	var def PolicyDefinition

	decoder := yaml.NewDecoder(reader)
	if err := decoder.Decode(&def); err != nil {
		return nil, err
	}

	return sanitizePolicyDefinition(def, name, source)
}
