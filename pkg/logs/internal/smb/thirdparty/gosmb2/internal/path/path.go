// Package path provides SMB and DFS path normalization and validation
// shared by the SMB2 client and the DFS resolver. SMB pathnames always use
// backslash as the separator.
package path

import (
	"os"
	"strings"
	"unicode/utf8"
)

// Separator is the SMB path separator.
const Separator = '\\'

// IsSeparator reports whether c is an SMB path separator.
func IsSeparator(c uint8) bool {
	return c == Separator
}

// Base returns the last element of path. Trailing separators are removed.
func Base(path string) string {
	j := len(path)
	for j > 0 && IsSeparator(path[j-1]) {
		j--
	}

	if j == 0 {
		return ""
	}

	i := j - 1
	for i > 0 && !IsSeparator(path[i-1]) {
		i--
	}

	return path[i:j]
}

// Dir returns all but the last element of path, typically the path's
// directory.
func Dir(path string) string {
	if path == "" {
		return ""
	}

	i := len(path)
	for i > 0 && IsSeparator(path[i-1]) {
		i--
	}

	if i == 0 {
		return `\`
	}

	i--
	for i > 0 && !IsSeparator(path[i-1]) {
		i--
	}

	if i == 0 {
		return ""
	}

	i--
	for i > 0 && IsSeparator(path[i-1]) {
		i--
	}

	if i == 0 {
		return `\`
	}

	return path[:i]
}

// Split splits path immediately following the final separator, separating it
// into a directory and file name component.
func Split(path string) (dir, file string) {
	i := len(path) - 1
	for i >= 0 && !IsSeparator(path[i]) {
		i--
	}
	return path[:i+1], path[i+1:]
}

// Join joins path elements with the SMB separator and normalizes the result.
// Empty elements are ignored.
func Join(elem ...string) string {
	for i, e := range elem {
		if e != "" {
			return Normalize(strings.Join(elem[i:], string(Separator)))
		}
	}
	return ""
}

// ----------------------------------------------------------------------------
// UNC paths
// ----------------------------------------------------------------------------

// UNC is a parsed \\<server>\<share>[\<relpath>] path.
type UNC struct {
	Server  string
	Share   string
	RelPath string
}

// JoinUNC joins server, share, and optional relative path components into a
// canonical UNC path \\<server>\<share>[\<relpath>].
func JoinUNC(server, share string, elem ...string) string {
	base := `\\` + server + `\` + share
	if len(elem) == 0 {
		return base
	}
	rest := strings.TrimLeft(Normalize(strings.Join(elem, string(Separator))), `\`)
	if rest == "" {
		return base
	}
	return base + `\` + rest
}

// SplitAll splits path by SMB separators into its non-empty components.
func SplitAll(path string) []string {
	path = strings.ReplaceAll(path, `/`, `\`)
	path = strings.Trim(path, `\`)
	if path == "" {
		return nil
	}
	parts := strings.Split(path, `\`)
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SharePath returns the \\<server>\<share> prefix.
func (u UNC) SharePath() string {
	return JoinUNC(u.Server, u.Share)
}

// String reassembles the UNC path.
func (u UNC) String() string {
	return JoinUNC(u.Server, u.Share, u.RelPath)
}

// ParseUNC parses and validates an absolute UNC path. It returns os.ErrInvalid
// for any malformed path.
func ParseUNC(path string) (UNC, error) {
	server, share, relPath, ok := SplitUNC(path)
	if !ok {
		return UNC{}, os.ErrInvalid
	}
	return UNC{Server: server, Share: share, RelPath: relPath}, nil
}

// SplitUNC splits an absolute UNC path into its server, share, and remaining
// components. It checks path structure, leaving server-specific name and length
// restrictions to the server.
func SplitUNC(path string) (server, share, relPath string, ok bool) {
	if len(path) < 3 || path[0] != '\\' || path[1] != '\\' || path[2] == '\\' {
		return "", "", "", false
	}
	parts := strings.Split(path[2:], `\`)
	if len(parts) < 2 {
		return "", "", "", false
	}
	for _, part := range parts {
		if !validComponent(part) {
			return "", "", "", false
		}
	}
	return parts[0], parts[1], strings.Join(parts[2:], `\`), true
}

// validComponent prevents ambiguous path structure and lossy string encoding.
// Character sets and length limits specific to a server are not checked here.
func validComponent(name string) bool {
	return name != "" && name != "." && name != ".." &&
		utf8.ValidString(name) && !strings.ContainsAny(name, "\\/\x00")
}

// NormalizeUNC validates path as an absolute UNC path and returns its
// canonical \\<server>\<share>[\<relpath>] form.
func NormalizeUNC(path string) (string, error) {
	u, err := ParseUNC(Normalize(path))
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// NormalizePattern normalizes an SMB search pattern without converting separators.
// Unlike Normalize it does not collapse separators, because a pattern may
// contain trailing elements
// that are meaningful to the matcher.
func NormalizePattern(pattern string) string {
	for strings.HasPrefix(pattern, `.\`) {
		pattern = pattern[2:]
	}
	return pattern
}

// Normalize normalizes an SMB path without converting separators. It drops a
// leading ".\", and collapses redundant separators while preserving the
// leading run of separators that marks a UNC or absolute pathname. Dot
// components are preserved so ValidRelPath can still reject them.
func Normalize(path string) string {
	path = NormalizePattern(path)
	if path == "." {
		return ""
	}

	// A leading run of separators marks a UNC or absolute pathname and is
	// significant, so it must be preserved. Collapse any redundant separators
	// elsewhere: [MS-FSCC] 2.1.5 composes a pathname from one or more non-empty
	// components, so an empty component must not be sent over the wire.
	i := 0
	for i < len(path) && path[i] == '\\' {
		i++
	}
	prefix, rest := path[:i], path[i:]
	if !strings.Contains(rest, `\\`) && !strings.HasSuffix(rest, `\`) {
		return path
	}
	elems := strings.Split(rest, `\`)
	out := elems[:0]
	for _, elem := range elems {
		if elem != "" {
			out = append(out, elem)
		}
	}
	return prefix + strings.Join(out, `\`)
}

// ValidRelPath reports whether path is a valid share-relative path for encoding
// into an [MS-SMB2] CREATE name.
// It follows io/fs.ValidPath's rules with backslash separators, except that the
// root is represented by an empty string rather than ".".
func ValidRelPath(path string) bool {
	if !utf8.ValidString(path) {
		return false
	}
	if len(path) == 0 {
		return true
	}

	// [MS-FSCC] 2.1.5.1 forbids sending "." or ".." components on the wire,
	// and [MS-SMB2] 2.2.13 requires the CREATE name to conform to that
	// pathname format.
	for {
		i := 0
		for i < len(path) && path[i] != Separator {
			i++
		}
		elem := path[:i]
		if elem == "" || elem == "." || elem == ".." {
			return false
		}
		if i == len(path) {
			return true
		}
		path = path[i+1:]
	}
}

// NormalizeRelPath normalizes path and validates that it is a valid share-relative path.
func NormalizeRelPath(path string) (string, error) {
	path = Normalize(path)
	if !ValidRelPath(path) {
		return "", os.ErrInvalid
	}
	return path, nil
}

// ValidShareName reports whether name is a valid single share name component.
func ValidShareName(name string) bool {
	return validComponent(name)
}

// ----------------------------------------------------------------------------
// DFS referral paths
// ----------------------------------------------------------------------------

// ParseReferralTarget parses a DFS target into a public UNC path. DFS wire
// paths use one leading separator ([MS-DFSC] 2.2.1); public UNC paths use two.
// The remaining path structure is validated without normalization.
func ParseReferralTarget(path string) (UNC, error) {
	if len(path) >= 2 && IsSeparator(path[0]) && !IsSeparator(path[1]) {
		path = string(Separator) + path
	}
	return ParseUNC(path)
}

// NormalizeReferralPath normalizes a DFS referral path to exactly one leading
// separator, as required by [MS-DFSC] 2.2.1. An empty path is returned
// unchanged.
func NormalizeReferralPath(path string) string {
	if path == "" {
		return ""
	}
	return string(Separator) + strings.TrimLeft(path, `\`)
}

// ValidReferralPath reports whether path is a valid DFS referral RequestFileName
// ([MS-DFSC] 3.1.4.2): an empty path (DOMAIN), \<domain> or \\<domain> (DC),
// or \\<server>\<share>[\<path>...] (SYSVOL/ROOT/LINK).
func ValidReferralPath(path string) bool {
	if path == "" {
		return true
	}
	if path[0] != '\\' {
		return false
	}
	if strings.HasPrefix(path, `\\`) {
		if len(path) >= 3 && path[2] == '\\' {
			return false
		}
		for part := range strings.SplitSeq(path[2:], `\`) {
			if !validComponent(part) || strings.ContainsRune(part, ':') {
				return false
			}
		}
		return true
	}
	// Single leading backslash: exactly one non-empty component.
	part := path[1:]
	return validComponent(part) && !strings.ContainsRune(part, ':')
}

// CutPrefix reports whether path begins with prefix (component-wise, case-insensitively).
// If it does, suffix is the remainder including its leading separator; otherwise ok is false.
func CutPrefix(path, prefix string) (suffix string, ok bool) {
	p := strings.Trim(path, `\`)
	pref := strings.Trim(prefix, `\`)
	if pref == "" {
		return "", false
	}
	for {
		component, rest, more := strings.Cut(p, `\`)
		prefixComponent, prefixRest, prefixMore := strings.Cut(pref, `\`)
		if !strings.EqualFold(component, prefixComponent) {
			return "", false
		}
		if !prefixMore {
			return p[len(component):], true
		}
		if !more {
			return "", false
		}
		p, pref = rest, prefixRest
	}
}

// AppendReferralSuffix combines a referral target with its unconsumed path suffix.
func AppendReferralSuffix(target, suffix string) string {
	if suffix == "" {
		return target
	}
	target = strings.TrimRight(ToPublicUNC(target), `\`)
	if !strings.HasPrefix(suffix, `\`) {
		suffix = `\` + suffix
	}
	return target + suffix
}

// ToPublicUNC converts a DFS wire path to public UNC form without cleaning components.
func ToPublicUNC(path string) string {
	path = strings.TrimLeft(path, `\`)
	return `\\` + path
}

// EqualReferralPath compares public and wire referral paths case-insensitively.
func EqualReferralPath(a, b string) bool {
	return strings.EqualFold(strings.TrimLeft(a, `\`), strings.TrimLeft(b, `\`))
}
