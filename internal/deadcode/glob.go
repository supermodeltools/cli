package deadcode

import (
	"path/filepath"
	"strings"
)

// matchGlob reports whether filePath matches the glob pattern.
// Supports *, ?, [...] within a single path segment (via filepath.Match)
// and ** to match zero or more path segments.
//
// Examples:
//
//	matchGlob("dist/**", "dist/index.js")         → true
//	matchGlob("**/generated/**", "src/generated/api.go") → true
//	matchGlob("**/*.test.ts", "src/foo.test.ts")  → true
func matchGlob(pattern, filePath string) bool {
	pattern = filepath.ToSlash(pattern)
	filePath = filepath.ToSlash(filePath)
	return matchSegments(strings.Split(pattern, "/"), strings.Split(filePath, "/"))
}

func matchSegments(pat, path []string) bool {
	for len(pat) > 0 {
		seg := pat[0]
		pat = pat[1:]

		if seg == "**" {
			// ** at the end matches one or more remaining segments (mirrors minimatch behaviour).
			if len(pat) == 0 {
				return len(path) > 0
			}
			// Try consuming 0, 1, 2, … path segments before resuming.
			for i := 0; i <= len(path); i++ {
				if matchSegments(pat, path[i:]) {
					return true
				}
			}
			return false
		}

		if len(path) == 0 {
			return false
		}

		ok, err := filepath.Match(seg, path[0])
		if err != nil || !ok {
			return false
		}
		path = path[1:]
	}
	return len(path) == 0
}
