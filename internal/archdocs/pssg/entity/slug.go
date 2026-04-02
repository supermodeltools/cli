package entity

import (
	"regexp"
	"strings"
)

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// ToSlug converts a string to a URL-safe slug.
// Lowercase, replace non-alphanumeric with hyphens, trim leading/trailing hyphens.
func ToSlug(s string) string {
	s = strings.ToLower(s)
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
