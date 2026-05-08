package liftoff

import (
	"errors"
	"regexp"
	"strings"
)

// kebabRe matches valid feature names: lowercase letters, digits, single dashes.
// Disallows leading/trailing dashes, double dashes, uppercase, underscores.
var kebabRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Normalize strips a leading "liftoff-" prefix (any case) and trims whitespace.
// Returns the canonical name. Does not validate; pair with Validate.
func Normalize(input string) string {
	s := strings.TrimSpace(input)
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "liftoff-") {
		s = s[len("liftoff-"):]
	}
	return strings.ToLower(s)
}

// Validate returns an error if name is not a valid kit feature name.
func Validate(name string) error {
	if name == "" {
		return errors.New("name is empty")
	}
	if name == "master" || name == "main" {
		return errors.New("name conflicts with main branch")
	}
	if len(name) > 60 {
		return errors.New("name too long (max 60 chars)")
	}
	if !kebabRe.MatchString(name) {
		return errors.New("name must be kebab-case: lowercase letters, digits, single dashes")
	}
	return nil
}

// NormalizeAndValidate is the convenience pair.
func NormalizeAndValidate(input string) (string, error) {
	n := Normalize(input)
	return n, Validate(n)
}
