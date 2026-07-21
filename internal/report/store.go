// SPDX-License-Identifier: Apache-2.0

package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dsgnr/datum/internal/statedir"
)

// Keep is how many passes are retained. Enough to see a pattern in a failure, few
// enough that the directory does not need managing.
const Keep = 20

// Names sort lexically in time order, which is what makes finding the latest a
// directory listing rather than a read of every file.
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

	path := filepath.Join(dir, r.FinishedAt.UTC().Format(nameLayout)+".json")
	if err := writeAtomic(path, body); err != nil {
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
	sort.Strings(out)
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

// writeAtomic keeps a half-written report from ever being read as a whole one.
func writeAtomic(path string, body []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".report-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())

	if err := temp.Chmod(statedir.FileMode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
