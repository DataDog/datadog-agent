// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// findPredicate represents a single test in a find expression.
type findPredicate struct {
	match  func(path string, info os.FileInfo, depth int) bool
	negate bool
}

func (r *Runner) builtinFind(args []string) error {
	// Parse search paths (arguments before the first flag starting with -).
	var searchPaths []string
	i := 0
	for i < len(args) {
		if strings.HasPrefix(args[i], "-") || args[i] == "!" || args[i] == "(" || args[i] == ")" {
			break
		}
		searchPaths = append(searchPaths, args[i])
		i++
	}
	if len(searchPaths) == 0 {
		searchPaths = []string{"."}
	}

	// Parse expressions.
	maxDepth := -1
	minDepth := 0
	var predicates []findPredicate
	negateNext := false

	for i < len(args) {
		a := args[i]
		switch a {
		case "-not", "!":
			negateNext = !negateNext
			i++
			continue

		case "-maxdepth":
			i++
			if i >= len(args) {
				return fmt.Errorf("find: missing argument to '-maxdepth'")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("find: invalid argument '%s' to '-maxdepth'", args[i])
			}
			maxDepth = n

		case "-mindepth":
			i++
			if i >= len(args) {
				return fmt.Errorf("find: missing argument to '-mindepth'")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("find: invalid argument '%s' to '-mindepth'", args[i])
			}
			minDepth = n

		case "-name":
			i++
			if i >= len(args) {
				return fmt.Errorf("find: missing argument to '-name'")
			}
			pattern := args[i]
			neg := negateNext
			negateNext = false
			predicates = append(predicates, findPredicate{
				negate: neg,
				match: func(path string, info os.FileInfo, depth int) bool {
					matched, _ := filepath.Match(pattern, info.Name())
					return matched
				},
			})

		case "-iname":
			i++
			if i >= len(args) {
				return fmt.Errorf("find: missing argument to '-iname'")
			}
			pattern := strings.ToLower(args[i])
			neg := negateNext
			negateNext = false
			predicates = append(predicates, findPredicate{
				negate: neg,
				match: func(path string, info os.FileInfo, depth int) bool {
					matched, _ := filepath.Match(pattern, strings.ToLower(info.Name()))
					return matched
				},
			})

		case "-type":
			i++
			if i >= len(args) {
				return fmt.Errorf("find: missing argument to '-type'")
			}
			typeChar := args[i]
			neg := negateNext
			negateNext = false
			predicates = append(predicates, findPredicate{
				negate: neg,
				match: func(path string, info os.FileInfo, depth int) bool {
					switch typeChar {
					case "f":
						return info.Mode().IsRegular()
					case "d":
						return info.IsDir()
					case "l":
						return info.Mode()&os.ModeSymlink != 0
					default:
						return false
					}
				},
			})

		case "-size":
			i++
			if i >= len(args) {
				return fmt.Errorf("find: missing argument to '-size'")
			}
			sizeSpec := args[i]
			neg := negateNext
			negateNext = false
			pred, err := parseSizePredicate(sizeSpec)
			if err != nil {
				return fmt.Errorf("find: invalid argument '%s' to '-size': %w", sizeSpec, err)
			}
			predicates = append(predicates, findPredicate{negate: neg, match: pred})

		case "-mtime":
			i++
			if i >= len(args) {
				return fmt.Errorf("find: missing argument to '-mtime'")
			}
			spec := args[i]
			neg := negateNext
			negateNext = false
			pred, err := parseTimePredicate(spec, 24*time.Hour)
			if err != nil {
				return fmt.Errorf("find: invalid argument '%s' to '-mtime': %w", spec, err)
			}
			predicates = append(predicates, findPredicate{negate: neg, match: pred})

		case "-mmin":
			i++
			if i >= len(args) {
				return fmt.Errorf("find: missing argument to '-mmin'")
			}
			spec := args[i]
			neg := negateNext
			negateNext = false
			pred, err := parseTimePredicate(spec, time.Minute)
			if err != nil {
				return fmt.Errorf("find: invalid argument '%s' to '-mmin': %w", spec, err)
			}
			predicates = append(predicates, findPredicate{negate: neg, match: pred})

		case "-path":
			i++
			if i >= len(args) {
				return fmt.Errorf("find: missing argument to '-path'")
			}
			pattern := args[i]
			neg := negateNext
			negateNext = false
			predicates = append(predicates, findPredicate{
				negate: neg,
				match: func(path string, info os.FileInfo, depth int) bool {
					matched, _ := filepath.Match(pattern, path)
					return matched
				},
			})

		case "-empty":
			neg := negateNext
			negateNext = false
			predicates = append(predicates, findPredicate{
				negate: neg,
				match: func(path string, info os.FileInfo, depth int) bool {
					if info.IsDir() {
						entries, err := os.ReadDir(path)
						return err == nil && len(entries) == 0
					}
					return info.Size() == 0
				},
			})

		case "-newer":
			i++
			if i >= len(args) {
				return fmt.Errorf("find: missing argument to '-newer'")
			}
			refPath := args[i]
			if !filepath.IsAbs(refPath) {
				refPath = filepath.Join(r.dir, refPath)
			}
			refInfo, err := os.Stat(refPath)
			if err != nil {
				return fmt.Errorf("find: cannot stat '%s': %w", args[i], err)
			}
			refTime := refInfo.ModTime()
			neg := negateNext
			negateNext = false
			predicates = append(predicates, findPredicate{
				negate: neg,
				match: func(path string, info os.FileInfo, depth int) bool {
					return info.ModTime().After(refTime)
				},
			})

		case "-print":
			// -print is the default action; we ignore it.

		default:
			return fmt.Errorf("find: predicate '%s' is not allowed", a)
		}

		i++
	}

	// Walk search paths.
	for _, searchPath := range searchPaths {
		absSearchPath := searchPath
		if !filepath.IsAbs(absSearchPath) {
			absSearchPath = filepath.Join(r.dir, absSearchPath)
		}

		err := filepath.Walk(absSearchPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Fprintf(r.stderr, "find: '%s': %v\n", path, err)
				return nil
			}

			// Calculate depth relative to search path.
			relPath, _ := filepath.Rel(absSearchPath, path)
			depth := 0
			if relPath != "." {
				depth = strings.Count(relPath, string(os.PathSeparator)) + 1
			}

			// Enforce maxdepth.
			if maxDepth >= 0 && depth > maxDepth {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// Enforce mindepth.
			if depth < minDepth {
				return nil
			}

			// Build the display path relative to the search path argument.
			displayPath := filepath.Join(searchPath, relPath)

			// Check all predicates (AND logic).
			// For -type l, use Lstat to detect symlinks.
			linfo := info
			if info.Mode()&os.ModeSymlink != 0 {
				linfo = info
			} else {
				li, lerr := os.Lstat(path)
				if lerr == nil {
					linfo = li
				}
			}

			for _, pred := range predicates {
				result := pred.match(path, linfo, depth)
				if pred.negate {
					result = !result
				}
				if !result {
					return nil
				}
			}

			fmt.Fprintln(r.stdout, displayPath)
			return nil
		})
		if err != nil {
			fmt.Fprintf(r.stderr, "find: %v\n", err)
			r.exitCode = 1
			return nil
		}
	}

	r.exitCode = 0
	return nil
}

func parseSizePredicate(spec string) (func(string, os.FileInfo, int) bool, error) {
	if len(spec) == 0 {
		return nil, fmt.Errorf("empty size specification")
	}

	// Parse prefix: +, -, or exact.
	compare := 0 // 0=exact, 1=greater, -1=less
	s := spec
	if s[0] == '+' {
		compare = 1
		s = s[1:]
	} else if s[0] == '-' {
		compare = -1
		s = s[1:]
	}

	// Parse suffix for units.
	multiplier := int64(512) // default: 512-byte blocks
	if len(s) > 0 {
		switch s[len(s)-1] {
		case 'c':
			multiplier = 1
			s = s[:len(s)-1]
		case 'w':
			multiplier = 2
			s = s[:len(s)-1]
		case 'k':
			multiplier = 1024
			s = s[:len(s)-1]
		case 'M':
			multiplier = 1024 * 1024
			s = s[:len(s)-1]
		case 'G':
			multiplier = 1024 * 1024 * 1024
			s = s[:len(s)-1]
		}
	}

	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid number: %s", spec)
	}
	targetSize := n * multiplier

	return func(path string, info os.FileInfo, depth int) bool {
		size := info.Size()
		switch compare {
		case 1:
			return size > targetSize
		case -1:
			return size < targetSize
		default:
			return size == targetSize
		}
	}, nil
}

func parseTimePredicate(spec string, unit time.Duration) (func(string, os.FileInfo, int) bool, error) {
	if len(spec) == 0 {
		return nil, fmt.Errorf("empty time specification")
	}

	compare := 0 // 0=exact, 1=older than, -1=newer than
	s := spec
	if s[0] == '+' {
		compare = 1
		s = s[1:]
	} else if s[0] == '-' {
		compare = -1
		s = s[1:]
	}

	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid number: %s", spec)
	}

	now := time.Now()

	return func(path string, info os.FileInfo, depth int) bool {
		age := now.Sub(info.ModTime())
		ageUnits := int64(age / unit)
		switch compare {
		case 1:
			return ageUnits > n
		case -1:
			return ageUnits < n
		default:
			return ageUnits == n
		}
	}, nil
}
