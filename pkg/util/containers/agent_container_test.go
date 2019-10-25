// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package containers

import "testing"

func Test_parseContainerID(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name:    "docker cgroup",
			path:    "./test/validCgroupDocker",
			want:    "5b9628e9c82b955fb6bfcfe8c12e345c80832943ff344a1b6128fe8cbfeb2059",
			wantErr: false,
		},
		{
			name:    "ecs cgroup",
			path:    "./test/validCgroupECS",
			want:    "c08d54707f4323b51f0361d80d7526cce7e356a71dbd3ad5b55959864566b7f3",
			wantErr: false,
		},
		{
			name:    "invalid cgroup",
			path:    "./test/invalidCgroup",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid path",
			path:    "./test/fileNotFound",
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseContainerID(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseContainerID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseContainerID() = %v, want %v", got, tt.want)
			}
		})
	}
}
