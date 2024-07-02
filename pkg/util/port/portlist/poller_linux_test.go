// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package portlist

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go4.org/mem"
)

func TestFieldIndex(t *testing.T) {
	tests := []struct {
		in    string
		field int
		want  int
	}{
		{"foo", 0, 0},
		{"  foo", 0, 2},
		{"foo  bar", 1, 5},
		{" foo  bar", 1, 6},
		{" foo  bar", 2, -1},
		{" foo  bar ", 2, -1},
		{" foo  bar x", 2, 10},
		{"  1: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 34062 1 0000000000000000 100 0 0 10 0",
			2, 19},
	}
	for _, tt := range tests {
		if got := fieldIndex([]byte(tt.in), tt.field); got != tt.want {
			t.Errorf("fieldIndex(%q, %v) = %v; want %v", tt.in, tt.field, got, tt.want)
		}
	}
}

func TestParsePorts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		file string
		want map[string]*portMeta
	}{
		{
			name: "empty",
			in:   "header line (ignored)\n",
			want: map[string]*portMeta{},
		},
		{
			name: "ipv4",
			file: "tcp",
			in: `header line
  0: 0100007F:0277 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 22303 1 0000000000000000 100 0 0 10 0
  1: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 34062 1 0000000000000000 100 0 0 10 0
  2: 5501A8C0:ADD4 B25E9536:01BB 01 00000000:00000000 02:00000B2B 00000000  1000        0 155276677 2 0000000000000000 22 4 30 10 -1
`,
			want: map[string]*portMeta{
				"socket:[34062]": {
					port: Port{Proto: "tcp", Port: 22},
				},
			},
		},
		{
			name: "ipv6",
			file: "tcp6",
			in: `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:0277 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 35720 1 0000000000000000 100 0 0 10 0
   1: 00000000000000000000000000000000:1F91 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 142240557 1 0000000000000000 100 0 0 10 0
   2: 00000000000000000000000000000000:0016 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 34064 1 0000000000000000 100 0 0 10 0
   3: 69050120005716BC64906EBE009ECD4D:D506 0047062600000000000000006E171268:01BB 01 00000000:00000000 02:0000009E 00000000  1000        0 151042856 2 0000000000000000 21 4 28 10 -1
`,
			want: map[string]*portMeta{
				"socket:[142240557]": {
					port: Port{Proto: "tcp", Port: 8081},
				},
				"socket:[34064]": {
					port: Port{Proto: "tcp", Port: 22},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.NewBufferString(tt.in)
			r := bufio.NewReader(buf)
			file := "tcp"
			if tt.file != "" {
				file = tt.file
			}
			li := newLinuxImplBase(false)
			err := li.parseProcNetFile(r, file)
			if err != nil {
				t.Fatal(err)
			}
			for _, pm := range tt.want {
				pm.keep = true
				pm.needsProcName = true
			}
			if diff := cmp.Diff(li.known, tt.want, cmp.AllowUnexported(Port{}), cmp.AllowUnexported(portMeta{})); diff != "" {
				t.Errorf("unexpected parsed ports (-got+want):\n%s", diff)
			}
		})
	}
}

func BenchmarkParsePorts(b *testing.B) {
	b.ReportAllocs()

	var contents bytes.Buffer
	contents.WriteString(`  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:0277 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 35720 1 0000000000000000 100 0 0 10 0
   1: 00000000000000000000000000000000:1F91 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 142240557 1 0000000000000000 100 0 0 10 0
   2: 00000000000000000000000000000000:0016 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 34064 1 0000000000000000 100 0 0 10 0
`)
	for i := 0; i < 50000; i++ {
		contents.WriteString("   3: 69050120005716BC64906EBE009ECD4D:D506 0047062600000000000000006E171268:01BB 01 00000000:00000000 02:0000009E 00000000  1000        0 151042856 2 0000000000000000 21 4 28 10 -1\n")
	}

	li := newLinuxImplBase(false)

	r := bytes.NewReader(contents.Bytes())
	br := bufio.NewReader(&contents)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Seek(0, io.SeekStart)
		br.Reset(r)
		err := li.parseProcNetFile(br, "tcp6")
		if err != nil {
			b.Fatal(err)
		}
		if len(li.known) != 2 {
			b.Fatalf("wrong results; want 2 parsed got %d", len(li.known))
		}
	}
}

func BenchmarkFindProcessNames(b *testing.B) {
	b.ReportAllocs()
	li := &linuxImpl{}
	need := map[string]*portMeta{
		"something-we'll-never-find": new(portMeta),
	}
	for i := 0; i < b.N; i++ {
		if err := li.findProcessNames(need); err != nil {
			b.Fatal(err)
		}
	}
}

func TestWalkShallow(t *testing.T) {
	d := t.TempDir()

	t.Run("basics", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(d, "foo"), []byte("1"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "bar"), []byte("22"), 0400); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(d, "baz"), 0777); err != nil {
			t.Fatal(err)
		}

		var got []string
		if err := walkShallow(mem.S(d), func(name mem.RO, de os.DirEntry) error {
			var size int64
			if fi, err := de.Info(); err != nil {
				t.Errorf("Info stat error on %q: %v", de.Name(), err)
			} else if !fi.IsDir() {
				size = fi.Size()
			}
			got = append(got, fmt.Sprintf("%q %q dir=%v type=%d size=%v", name.StringCopy(), de.Name(), de.IsDir(), de.Type(), size))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		sort.Strings(got)
		want := []string{
			`"bar" "bar" dir=false type=0 size=2`,
			`"baz" "baz" dir=true type=2147483648 size=0`,
			`"foo" "foo" dir=false type=0 size=1`,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("mismatch:\n got %#q\nwant %#q", got, want)
		}
	})

	t.Run("err_not_exist", func(t *testing.T) {
		err := walkShallow(mem.S(filepath.Join(d, "not_exist")), func(name mem.RO, de os.DirEntry) error {
			return nil
		})
		if !os.IsNotExist(err) {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestArgvSubject(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{
			in:   nil,
			want: "",
		},
		{
			in:   []string{"/usr/bin/sshd"},
			want: "sshd",
		},
		{
			in:   []string{"/bin/mono"},
			want: "mono",
		},
		{
			in:   []string{"/nix/store/x2cw2xjw98zdysf56bdlfzsr7cyxv0jf-mono-5.20.1.27/bin/mono", "/bin/exampleProgram.exe"},
			want: "exampleProgram",
		},
		{
			in:   []string{"/bin/mono", "/sbin/exampleProgram.bin"},
			want: "exampleProgram.bin",
		},
		{
			in:   []string{"/usr/bin/sshd_config [listener] 1 of 10-100 startups"},
			want: "sshd_config",
		},
		{
			in:   []string{"/usr/bin/sshd [listener] 0 of 10-100 startups"},
			want: "sshd",
		},
		{
			in:   []string{"/opt/aws/bin/eic_run_authorized_keys %u %f -o AuthorizedKeysCommandUser ec2-instance-connect [listener] 0 of 10-100 startups"},
			want: "eic_run_authorized_keys",
		},
		{
			in:   []string{"/usr/bin/nginx worker"},
			want: "nginx",
		},
	}

	for _, test := range tests {
		got := argvSubject(test.in...)
		if got != test.want {
			t.Errorf("argvSubject(%v) = %q, want %q", test.in, got, test.want)
		}
	}
}
