// SPDX-License-Identifier: Apache-2.0

package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dsgnr/datum/internal/statedir"
)

// Keep is how many passes are retained. Enough to see a pattern in a failure, few
// enough that the directory does not need managing.
const Keep = 20

// Keep the original millisecond prefix so existing reports sort with new ones.
// New names append the remaining nanoseconds and a sequence for identical times.
const nameLayout = "20060102T150405.000Z"

// Dir is where reports live under the state directory.
func Dir(stateDir string) string { return filepath.Join(stateDir, "reports") }

// Write stores a report and prunes the older ones.
func Write(stateDir string, r Report) (string, error) {
	if err := statedir.Ensure(stateDir); err != nil {
		return "", err
	}
	dir := Dir(stateDir)
	if err := statedir.Ensure(dir); err != nil {
		return "", err
	}

	body, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	body = append(body, '\n')

	prefix := filepath.Join(dir, fmt.Sprintf("%s-%06d",
		r.FinishedAt.UTC().Format(nameLayout), r.FinishedAt.Nanosecond()%int(time.Millisecond)))
	path, err := writeAtomic(prefix, body)
	if err != nil {
		return "", err
	}
	// A pruning failure does not invalidate the report that was just written.
	prune(dir, Keep)
	return path, nil
}

// Latest reads the most recent report. A host that has never run has none, which is not
// worth a sentinel error.
func Latest(stateDir string) (Report, bool, error) {
	names, err := list(Dir(stateDir))
	if err != nil || len(names) == 0 {
		return Report{}, false, err
	}
	r, err := read(filepath.Join(Dir(stateDir), names[len(names)-1]))
	if err != nil {
		return Report{}, false, err
	}
	return r, true, nil
}

// History reads the retained reports, newest first.
func History(stateDir string) ([]Report, error) {
	dir := Dir(stateDir)
	names, err := list(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Report, 0, len(names))
	for i := len(names) - 1; i >= 0; i-- {
		r, err := read(filepath.Join(dir, names[i]))
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func read(path string) (Report, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	var r Report
	if err := json.Unmarshal(body, &r); err != nil {
		return Report{}, fmt.Errorf("reading %s: %w", path, err)
	}
	return r, nil
}

func list(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			out = append(out, entry.Name())
		}
	}
	// Removing the extension puts a legacy timestamp before a new filename
	// with the same millisecond prefix. The dot in .json would sort after a dash.
	sort.Slice(out, func(i, j int) bool {
		return strings.TrimSuffix(out[i], ".json") < strings.TrimSuffix(out[j], ".json")
	})
	return out, nil
}

func prune(dir string, keep int) {
	names, err := list(dir)
	if err != nil || len(names) <= keep {
		return
	}
	for _, name := range names[:len(names)-keep] {
		os.Remove(filepath.Join(dir, name))
	}
}

// writeAtomic publishes a complete report without replacing an existing one.
func writeAtomic(prefix string, body []byte) (string, error) {
	temp, err := os.CreateTemp(filepath.Dir(prefix), ".report-")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())

	if err := temp.Chmod(statedir.FileMode); err != nil {
		temp.Close()
		return "", err
	}
	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}

	// Start after the highest retained sequence, not at the first gap: pruning
	// may already have removed earlier reports with exactly this timestamp.
	names, err := list(filepath.Dir(prefix))
	if err != nil {
		return "", err
	}
	var sequence uint64
	for _, name := range names {
		suffix, matches := strings.CutPrefix(name, filepath.Base(prefix)+"-")
		if !matches {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSuffix(suffix, ".json"), 10, 64)
		if err == nil && n >= sequence {
			if n == ^uint64(0) {
				return "", fmt.Errorf("report sequence exhausted for %s", prefix)
			}
			sequence = n + 1
		}
	}
	for {
		path := fmt.Sprintf("%s-%020d.json", prefix, sequence)
		// A hard link publishes the finished file atomically and fails if another
		// writer already claimed the name. Rename would silently replace it.
		err := os.Link(temp.Name(), path)
		if err == nil {
			return path, nil
		}
		if !os.IsExist(err) || sequence == ^uint64(0) {
			return "", err
		}
		sequence++
	}
}
