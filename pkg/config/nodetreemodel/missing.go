package nodetreemodel

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

type missingLeafImpl struct{}

var missingLeaf missingLeafImpl

func (m *missingLeafImpl) GetAny() (any, error) {
	return nil, fmt.Errorf("missing")
}

func (m *missingLeafImpl) GetBool() (bool, error) {
	return false, fmt.Errorf("missing")
}

func (m *missingLeafImpl) GetInt() (int, error) {
	return 0, fmt.Errorf("missing")
}

func (m *missingLeafImpl) GetFloat() (float64, error) {
	return 0.0, fmt.Errorf("missing")
}

func (m *missingLeafImpl) GetString() (string, error) {
	return "", fmt.Errorf("missing")
}

func (m *missingLeafImpl) GetTime() (time.Time, error) {
	return time.Time{}, fmt.Errorf("missing")
}

func (m *missingLeafImpl) GetDuration() (time.Duration, error) {
	return time.Duration(0), fmt.Errorf("missing")
}

func (m *missingLeafImpl) SetWithSource(interface{}, model.Source) error {
	return fmt.Errorf("missing")
}
