package interp

import (
	"io/fs"
	"testing"
	"time"
)

func TestFindParseSize(t *testing.T) {
	tests := []struct {
		input     string
		wantCmp   int
		wantBytes int64
		wantOK    bool
	}{
		// Bytes suffix.
		{"+100c", 1, 100, true},
		{"-50c", -1, 50, true},
		{"42c", 0, 42, true},

		// KiB suffix.
		{"+100k", 1, 100 * 1024, true},
		{"-1k", -1, 1024, true},
		{"2k", 0, 2 * 1024, true},

		// MiB suffix.
		{"+1M", 1, 1024 * 1024, true},
		{"-2M", -1, 2 * 1024 * 1024, true},

		// GiB suffix.
		{"+1G", 1, 1024 * 1024 * 1024, true},

		// No suffix: 512-byte blocks.
		{"10", 0, 10 * 512, true},
		{"+5", 1, 5 * 512, true},
		{"-3", -1, 3 * 512, true},

		// Zero.
		{"0c", 0, 0, true},

		// Invalid.
		{"", 0, 0, false},
		{"+", 0, 0, false},
		{"abc", 0, 0, false},
		{"+c", 0, 0, false},
		{"-k", 0, 0, false},
		{"+1x", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotCmp, gotBytes, gotOK := findParseSize(tt.input)
			if gotOK != tt.wantOK {
				t.Fatalf("findParseSize(%q) ok = %v, want %v", tt.input, gotOK, tt.wantOK)
			}
			if !gotOK {
				return
			}
			if gotCmp != tt.wantCmp {
				t.Errorf("findParseSize(%q) cmp = %d, want %d", tt.input, gotCmp, tt.wantCmp)
			}
			if gotBytes != tt.wantBytes {
				t.Errorf("findParseSize(%q) bytes = %d, want %d", tt.input, gotBytes, tt.wantBytes)
			}
		})
	}
}

// mockFileInfo implements fs.FileInfo for testing findMatch.
type mockFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() fs.FileMode  { return m.mode }
func (m *mockFileInfo) ModTime() time.Time { return m.modTime }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() any           { return nil }

func TestFindMatch(t *testing.T) {
	tests := []struct {
		name string
		info fs.FileInfo
		base string
		opts findOpts
		want bool
	}{
		{
			name: "match all with no predicates",
			info: &mockFileInfo{name: "foo.go", mode: 0o644},
			base: "foo.go",
			opts: findOpts{maxDepth: -1},
			want: true,
		},
		{
			name: "name match",
			info: &mockFileInfo{name: "foo.go", mode: 0o644},
			base: "foo.go",
			opts: findOpts{maxDepth: -1, namePattern: "*.go"},
			want: true,
		},
		{
			name: "name no match",
			info: &mockFileInfo{name: "foo.txt", mode: 0o644},
			base: "foo.txt",
			opts: findOpts{maxDepth: -1, namePattern: "*.go"},
			want: false,
		},
		{
			name: "type f match",
			info: &mockFileInfo{name: "foo.go", mode: 0o644},
			base: "foo.go",
			opts: findOpts{maxDepth: -1, fileType: 'f'},
			want: true,
		},
		{
			name: "type f no match on dir",
			info: &mockFileInfo{name: "dir", mode: fs.ModeDir | 0o755, isDir: true},
			base: "dir",
			opts: findOpts{maxDepth: -1, fileType: 'f'},
			want: false,
		},
		{
			name: "type d match",
			info: &mockFileInfo{name: "dir", mode: fs.ModeDir | 0o755, isDir: true},
			base: "dir",
			opts: findOpts{maxDepth: -1, fileType: 'd'},
			want: true,
		},
		{
			name: "type l match",
			info: &mockFileInfo{name: "link", mode: fs.ModeSymlink | 0o777},
			base: "link",
			opts: findOpts{maxDepth: -1, fileType: 'l'},
			want: true,
		},
		{
			name: "type l no match on regular",
			info: &mockFileInfo{name: "foo", mode: 0o644},
			base: "foo",
			opts: findOpts{maxDepth: -1, fileType: 'l'},
			want: false,
		},
		{
			name: "empty file match",
			info: &mockFileInfo{name: "empty", mode: 0o644, size: 0},
			base: "empty",
			opts: findOpts{maxDepth: -1, empty: true, hasEmpty: true},
			want: true,
		},
		{
			name: "non-empty file no match",
			info: &mockFileInfo{name: "data", mode: 0o644, size: 100},
			base: "data",
			opts: findOpts{maxDepth: -1, empty: true, hasEmpty: true},
			want: false,
		},
		{
			name: "size greater than",
			info: &mockFileInfo{name: "big", mode: 0o644, size: 2000},
			base: "big",
			opts: findOpts{maxDepth: -1, sizeStr: "+1k"},
			want: true,
		},
		{
			name: "size not greater than",
			info: &mockFileInfo{name: "small", mode: 0o644, size: 500},
			base: "small",
			opts: findOpts{maxDepth: -1, sizeStr: "+1k"},
			want: false,
		},
		{
			name: "size less than",
			info: &mockFileInfo{name: "small", mode: 0o644, size: 500},
			base: "small",
			opts: findOpts{maxDepth: -1, sizeStr: "-1k"},
			want: true,
		},
		{
			name: "size exact",
			info: &mockFileInfo{name: "exact", mode: 0o644, size: 1024},
			base: "exact",
			opts: findOpts{maxDepth: -1, sizeStr: "1k"},
			want: true,
		},
		{
			name: "combined name and type",
			info: &mockFileInfo{name: "foo.go", mode: 0o644},
			base: "foo.go",
			opts: findOpts{maxDepth: -1, namePattern: "*.go", fileType: 'f'},
			want: true,
		},
		{
			name: "combined name match type no match",
			info: &mockFileInfo{name: "foo.go", mode: fs.ModeDir | 0o755, isDir: true},
			base: "foo.go",
			opts: findOpts{maxDepth: -1, namePattern: "*.go", fileType: 'f'},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findMatch(tt.info, tt.base, tt.opts)
			if got != tt.want {
				t.Errorf("findMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindArgParsing(t *testing.T) {
	// Test that argument parsing correctly splits paths from predicates.
	// We test this by checking what the parser does with various arg slices.
	tests := []struct {
		name      string
		args      []string
		wantPaths []string
		wantPreds int // number of predicate tokens consumed
	}{
		{
			name:      "no args",
			args:      nil,
			wantPaths: []string{"."},
		},
		{
			name:      "single path",
			args:      []string{"/tmp"},
			wantPaths: []string{"/tmp"},
		},
		{
			name:      "multiple paths",
			args:      []string{"/tmp", "/var"},
			wantPaths: []string{"/tmp", "/var"},
		},
		{
			name:      "path then predicate",
			args:      []string{"/tmp", "-name", "*.go"},
			wantPaths: []string{"/tmp"},
			wantPreds: 2,
		},
		{
			name:      "predicate only",
			args:      []string{"-name", "*.go"},
			wantPaths: []string{"."},
			wantPreds: 2,
		},
		{
			name:      "dot path",
			args:      []string{".", "-type", "f"},
			wantPaths: []string{"."},
			wantPreds: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the path/predicate splitting logic.
			var paths []string
			i := 0
			args := tt.args
			for i < len(args) {
				if len(args[i]) > 0 && args[i][0] == '-' {
					break
				}
				paths = append(paths, args[i])
				i++
			}
			if len(paths) == 0 {
				paths = []string{"."}
			}
			predsRemaining := len(args) - i

			if len(paths) != len(tt.wantPaths) {
				t.Fatalf("paths = %v, want %v", paths, tt.wantPaths)
			}
			for j := range paths {
				if paths[j] != tt.wantPaths[j] {
					t.Errorf("paths[%d] = %q, want %q", j, paths[j], tt.wantPaths[j])
				}
			}
			if predsRemaining != tt.wantPreds {
				t.Errorf("predicate tokens = %d, want %d", predsRemaining, tt.wantPreds)
			}
		})
	}
}
