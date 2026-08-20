// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a Go duration that also accepts plain days, since an interval of 1d
// reads better in a configuration file than 24h and the standard parser refuses it.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var text string
	if err := node.Decode(&text); err != nil {
		return fmt.Errorf("line %d: a duration has to be a string such as 30m", node.Line)
	}
	parsed, err := ParseDuration(text)
	if err != nil {
		return fmt.Errorf("line %d: %w", node.Line, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) String() string { return time.Duration(d).String() }

// ParseDuration reads a duration, allowing a trailing d for whole days.
func ParseDuration(text string) (time.Duration, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("a duration cannot be empty")
	}
	if days, ok := strings.CutSuffix(text, "d"); ok {
		// Only a bare number of days, so 1d30m still goes to the standard parser
		// and keeps its meaning.
		if n, err := strconv.Atoi(days); err == nil {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	parsed, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("%q is not a duration, want something like 30m or 15m30s", text)
	}
	return parsed, nil
}

// Size is a byte count written the way the reference writes it, as 1GiB or 16MiB.
type Size int64

func (s *Size) UnmarshalYAML(node *yaml.Node) error {
	// A plain integer is a byte count, which is what somebody writing 1048576 means.
	var n int64
	if err := node.Decode(&n); err == nil {
		if n < 0 {
			return fmt.Errorf("line %d: a size cannot be negative", node.Line)
		}
		*s = Size(n)
		return nil
	}

	var text string
	if err := node.Decode(&text); err != nil {
		return fmt.Errorf("line %d: a size has to be a string such as 16MiB", node.Line)
	}
	parsed, err := ParseSize(text)
	if err != nil {
		return fmt.Errorf("line %d: %w", node.Line, err)
	}
	*s = parsed
	return nil
}

// units are binary because the reference writes GiB and MiB. The decimal spellings are
// accepted as the same value, since a fleet writing 1GB in this file means the limit,
// not a precise number of bytes.
var units = []struct {
	suffix string
	scale  int64
}{
	{"KiB", 1 << 10}, {"MiB", 1 << 20}, {"GiB", 1 << 30}, {"TiB", 1 << 40},
	{"KB", 1 << 10}, {"MB", 1 << 20}, {"GB", 1 << 30}, {"TB", 1 << 40},
	{"K", 1 << 10}, {"M", 1 << 20}, {"G", 1 << 30}, {"T", 1 << 40},
	{"B", 1},
}

// ParseSize reads a byte count with an optional binary unit.
func ParseSize(text string) (Size, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, fmt.Errorf("a size cannot be empty")
	}
	for _, unit := range units {
		digits, ok := cutSuffixFold(trimmed, unit.suffix)
		if !ok {
			continue
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(digits), 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("%q is not a size, want something like 16MiB", text)
		}
		return Size(n * float64(unit.scale)), nil
	}
	n, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%q is not a size, want something like 16MiB", text)
	}
	return Size(n), nil
}

func (s Size) String() string {
	for i := len(units) - 1; i >= 0; i-- {
		unit := units[i]
		if !strings.HasSuffix(unit.suffix, "iB") {
			continue
		}
		if int64(s) >= unit.scale && int64(s)%unit.scale == 0 {
			return strconv.FormatInt(int64(s)/unit.scale, 10) + unit.suffix
		}
	}
	return strconv.FormatInt(int64(s), 10) + "B"
}

func cutSuffixFold(text, suffix string) (string, bool) {
	if len(text) < len(suffix) {
		return "", false
	}
	if !strings.EqualFold(text[len(text)-len(suffix):], suffix) {
		return "", false
	}
	return text[:len(text)-len(suffix)], true
}
