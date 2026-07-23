// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"path/filepath"
	"strings"
)

// matchGlob reports whether a slash separated path matches a pattern.
//
// Within a segment the rules are filepath.Match's. A ** segment matches any number
// of segments, including none.
func matchGlob(pattern, path string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

func matchSegments(pattern, path []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			// Consume nothing, then one segment, then two, and so on.
			for i := 0; i <= len(path); i++ {
				if matchSegments(pattern[1:], path[i:]) {
					return true
				}
			}
			return false
		}
		if len(path) == 0 {
			return false
		}
		ok, err := filepath.Match(pattern[0], path[0])
		if err != nil || !ok {
			return false
		}
		pattern = pattern[1:]
		path = path[1:]
	}
	return len(path) == 0
}

// excluded reports whether the fleet's patterns skip a path. A pattern matching a
// directory excludes everything under it.
func excluded(patterns []string, rel string) bool {
	segments := strings.Split(rel, "/")
	for _, pattern := range patterns {
		for i := 1; i <= len(segments); i++ {
			if matchGlob(pattern, strings.Join(segments[:i], "/")) {
				return true
			}
		}
	}
	return false
}
