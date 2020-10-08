package filter

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagFilter_Filter(t *testing.T) {
	type args struct {
		tags []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "nil",
			args: args{},
			want: nil,
		},
		{
			name: "empty",
			args: args{[]string{}},
			want: []string{},
		},
		{
			name: "unchanged",
			args: args{[]string{"key:value"}},
			want: []string{"key:value"},
		},
		{
			name: "filtered",
			args: args{[]string{"key:drop_me"}},
			want: []string{},
		},
		{
			name: "mangled",
			args: args{[]string{"calling_service:foo/e84f81f89e3876b2b11db348e95c8ab056e134ac"}},
			want: []string{"calling_service:foo"},
		},
		{
			name: "keep match group that removes colon separator",
			args: args{[]string{"keep:without colon"}},
			want: []string{"keep:without colon"},
		},
		{
			name: "drop untitled match group",
			args: args{[]string{"drop:untitled match group"}},
			want: []string{},
		},
		{
			name: "drop after mangled",
			args: args{[]string{"drop:after mangled"}},
			want: []string{},
		},
		{
			name: "multi tag filter",
			args: args{[]string{
				"calling_service:foo/e84f81f89e3876b2b11db348e95c8ab056e134ac",
				"key:drop_me",
				"key:value",
				"version:",
				"version:1.0.0",
				"request_id:CA761232-ED42-11CE-BACD-00AA0057B223",
				"request_source:CA761232-ED42-11CE-BACD-00AA0057B223",
				"request_source:genCA761232-ED42-11CE-BACD-00AA0057B223",
			}},
			want: []string{
				"calling_service:foo",
				"key:value",
				"version:1.0.0",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tf, err := NewTagFilter([]string{
				// A match group named "Keep" will remove anything not matched by the match group
				"^(?P<Keep>calling_service:[^/]+)",
				"(?P<Keep>without colon)",
				"(?P<Keep>drop:after)",
				":$", // remove empty string tag values
				"^drop:after$",
				"(untitled match group)",
				`[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}`, // uuid
				"drop_me",
			})
			assert.NoError(t, err)
			if got := tf.Filter(tt.args.tags); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Filter() = %v, want %v", got, tt.want)
			}
		})
	}
}
