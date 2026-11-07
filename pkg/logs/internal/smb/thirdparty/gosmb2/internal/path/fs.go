package path

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// ToPOSIXPath converts SMB separators to io/fs separators without cleaning.
func ToPOSIXPath(name string) string { return strings.ReplaceAll(name, `\`, "/") }

// ToSMBPath converts io/fs separators to SMB separators without cleaning.
func ToSMBPath(name string) string { return strings.ReplaceAll(name, "/", `\`) }

// ValidPosixPath rejects names that cannot be represented as SMB path components.
func ValidPosixPath(name string) bool { return fs.ValidPath(name) && !strings.ContainsRune(name, '\\') }

// PosixPathToUNC maps a virtual server/share/path to its UNC path.
func PosixPathToUNC(name string) string { return `\\` + ToSMBPath(name) }

// GlobFS expands slash-separated patterns using path.Match syntax. Search
// receives a concrete directory and the original single-component pattern,
// and returns candidate basenames. Candidates are matched locally, allowing
// a server to use a broader search pattern without changing the result.
func GlobFS(pattern string, lstat func(string) (fs.FileInfo, error), search func(string, string) ([]string, error)) ([]string, error) {
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, err
	}
	return globFS(pattern, 0, lstat, search)
}

func globFS(pattern string, depth int, lstat func(string) (fs.FileInfo, error), search func(string, string) ([]string, error)) ([]string, error) {
	if depth >= 10000 {
		return nil, path.ErrBadPattern
	}
	if !strings.ContainsAny(pattern, `*?[\`) {
		if !ValidPosixPath(pattern) {
			return nil, nil
		}
		if _, err := lstat(pattern); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			return nil, nil
		}
		return []string{pattern}, nil
	}
	dir, leaf := path.Split(pattern)
	// Remove only path.Split's trailing separator, as generic fs.Glob does.
	// Concrete lookup paths are validated separately; do not clean literal
	// invalid paths or repeatedly clean whole prefixes in deep patterns.
	if dir == "" {
		dir = "."
	} else {
		dir = strings.TrimSuffix(dir, "/")
	}
	dirs := []string{dir}
	if strings.ContainsAny(dir, `*?[\`) {
		var err error
		dirs, err = globFS(dir, depth+1, lstat, search)
		if err != nil {
			return nil, err
		}
	}
	var matches []string
	for _, dir := range dirs {
		if !ValidPosixPath(dir) {
			continue
		}
		names, err := search(dir, leaf)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			matched, err := path.Match(leaf, name)
			if err != nil {
				return nil, err
			}
			if matched {
				matches = append(matches, path.Join(dir, name))
			}
		}
	}
	slices.Sort(matches)
	return matches, nil
}

// SMBSearchPattern converts one io/fs pattern component into an SMB pattern
// for candidate filtering. path.Match matches '?' and character classes by rune,
// while Windows matches '?' by UTF-16 code unit: '中' occupies one unit, but '😀'
// occupies two. Neither '?' nor '??' can cover both as a single rune match.
// Use '*' for these positions and escaped characters to avoid losing candidates,
// then match results against the original pattern using path.Match to remove
// false positives.
func SMBSearchPattern(pattern string) string {
	var out strings.Builder
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '?':
			out.WriteByte('*')
		case '\\':
			i++
			out.WriteByte('*')
		case '[':
			for i++; i < len(runes); i++ {
				if runes[i] == '\\' {
					i++
					continue
				}
				if runes[i] == ']' {
					break
				}
			}
			out.WriteByte('*')
		default:
			out.WriteRune(runes[i])
		}
	}
	return out.String()
}

// JoinPOSIXPath joins and cleans io/fs path elements.
func JoinPOSIXPath(elem ...string) string { return path.Join(elem...) }

// BasePOSIXPath returns the final element of an io/fs path.
func BasePOSIXPath(name string) string { return path.Base(name) }

// HasPOSIXSeparator reports whether name contains an io/fs separator.
func HasPOSIXSeparator(name string) bool { return strings.ContainsRune(name, '/') }
