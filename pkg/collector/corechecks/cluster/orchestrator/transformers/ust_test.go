package transformers

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func TestRetrieveUST(t *testing.T) {
	cfg := config.Mock(t)
	cfg.Set("env", "staging")
	cfg.Set(tagKeyService, "not-applied")
	cfg.Set(tagKeyVersion, "not-applied")

	tests := []struct {
		name   string
		labels map[string]string
		want   []string
	}{
		{
			name:   "label contains ust, labels ust takes precedence",
			labels: map[string]string{kubernetes.EnvTagLabelKey: "prod", kubernetes.VersionTagLabelKey: "123", kubernetes.ServiceTagLabelKey: "app"},
			want:   []string{"env:prod", "version:123", "service:app"},
		},
		{
			name:   "label does not contain env, takes from config",
			labels: map[string]string{},
			want:   []string{"env:staging"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RetrieveUnifiedServiceTags(tt.labels); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RetrieveUnifiedServiceTags() = %v, want %v", got, tt.want)
			}
		})
	}
}
