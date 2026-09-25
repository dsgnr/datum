// SPDX-License-Identifier: Apache-2.0

// Package discover finds the documents in a repository and works out which layer
// each resource belongs to.
package discover

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dsgnr/datum/internal/document"
)

// Result is everything found under one fleet root.
type Result struct {
	// Root holds the Fleet document. Paths in the documents are relative to it.
	Root string
	Set  document.Set
}

// Walk reads every document under dir and associates each resource with its
// nearest Layer.
func Walk(dir string) (Result, error) {
	files, err := yamlFiles(dir)
	if err != nil {
		return Result{}, err
	}

	parsed := make(map[string]document.Parsed, len(files))
	problems := make(map[string]document.Errors, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return Result{}, err
		}
		// The fleet root is not known until the Fleet document turns up, so parse
		// against this path and rewrite it afterwards.
		rel, err := filepath.Rel(dir, file)
		if err != nil {
			return Result{}, err
		}
		out, errs := document.ParseFile(filepath.ToSlash(rel), data)
		parsed[file] = out
		problems[file] = errs
	}

	root, fleet, err := findFleetRoot(dir, parsed)
	if err != nil {
		return Result{}, err
	}

	result := Result{Root: root, Set: document.Set{Fleet: fleet}}
	var errs document.Errors

	for _, file := range files {
		rel, err := filepath.Rel(root, file)
		if err != nil {
			return Result{}, err
		}
		rel = filepath.ToSlash(rel)
		// Outside the fleet root or excluded, so not this fleet's problem.
		if strings.HasPrefix(rel, "../") {
			continue
		}
		if excluded(fleet.Exclude, rel) {
			continue
		}

		fileErrs := problems[file]
		errs.Extend(retarget(fileErrs, rel))
		collect(&result.Set, parsed[file], rel)
	}

	sortSet(&result.Set)
	assignLayers(&result.Set, &errs)
	if err := errs.Err(); err != nil {
		return result, err
	}
	return result, nil
}

// Files lists the documents under a fleet root that discovery reads, as slash
// separated paths relative to root.
//
// Anything rewriting a repository has to see the same files the agent does, which is
// why the walk and the exclusion rules are shared rather than restated.
func Files(root string, exclude []string) ([]string, error) {
	found, err := yamlFiles(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, file := range found {
		rel, err := filepath.Rel(root, file)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		if excluded(exclude, rel) {
			continue
		}
		out = append(out, rel)
	}
	return out, nil
}

// yamlFiles lists the YAML under dir, skipping directories that never hold fleet
// content.
func yamlFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipDir(entry.Name()) && path != dir {
				return fs.SkipDir
			}
			return nil
		}
		switch filepath.Ext(entry.Name()) {
		case ".yaml", ".yml":
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func skipDir(name string) bool {
	switch name {
	case ".git", ".github", "node_modules", "site", ".venv", ".cache", "bin":
		return true
	}
	return false
}

// findFleetRoot locates the single Fleet document. Its directory is the fleet root.
func findFleetRoot(dir string, parsed map[string]document.Parsed) (string, document.Fleet, error) {
	type found struct {
		file  string
		fleet document.Fleet
	}
	var all []found
	for file, out := range parsed {
		for _, fleet := range out.Fleets {
			all = append(all, found{file: file, fleet: fleet})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].file < all[j].file })

	switch len(all) {
	case 0:
		return "", document.Fleet{}, fmt.Errorf("no Fleet document found under %s", dir)
	case 1:
		return filepath.Dir(all[0].file), all[0].fleet, nil
	default:
		var names []string
		for _, f := range all {
			names = append(names, f.file)
		}
		return "", document.Fleet{}, fmt.Errorf("found %d Fleet documents, expected one:\n  %s",
			len(all), strings.Join(names, "\n  "))
	}
}

// retarget rewrites each error's path to be relative to the fleet root.
func retarget(errs document.Errors, rel string) document.Errors {
	var out document.Errors
	for _, e := range errs.List() {
		out.Add(document.Position{File: rel, Line: e.Position.Line}, "%s", e.Msg)
	}
	return out
}

func collect(set *document.Set, parsed document.Parsed, rel string) {
	dir := relDir(filepath.Dir(rel))

	for _, host := range parsed.Hosts {
		host.Position.File = rel
		set.Hosts = append(set.Hosts, host)
	}
	for _, layer := range parsed.Layers {
		layer.Position.File = rel
		layer.Dir = dir
		set.Layers = append(set.Layers, layer)
	}
	for _, resource := range parsed.Resources {
		resource.Position.File = rel
		set.Resources = append(set.Resources, resource)
	}
}

// relDir gives the slash separated form the documents use, with the fleet root
// itself written as "".
func relDir(dir string) string {
	dir = filepath.ToSlash(dir)
	if dir == "." {
		return ""
	}
	return dir
}

func sortSet(set *document.Set) {
	sort.SliceStable(set.Hosts, func(i, j int) bool { return set.Hosts[i].Name < set.Hosts[j].Name })
	sort.SliceStable(set.Layers, func(i, j int) bool {
		if set.Layers[i].Precedence != set.Layers[j].Precedence {
			return set.Layers[i].Precedence < set.Layers[j].Precedence
		}
		return set.Layers[i].Position.File < set.Layers[j].Position.File
	})
	sort.SliceStable(set.Resources, func(i, j int) bool {
		if set.Resources[i].Type != set.Resources[j].Type {
			return set.Resources[i].Type < set.Resources[j].Type
		}
		return set.Resources[i].Name < set.Resources[j].Name
	})
}

// assignLayers gives every resource its nearest Layer.
func assignLayers(set *document.Set, errs *document.Errors) {
	byDir := map[string]document.Layer{}
	for _, layer := range set.Layers {
		if existing, clash := byDir[layer.Dir]; clash {
			errs.Add(layer.Position, "two Layer documents in the same directory, %q and %q, so resources here would be ambiguous",
				existing.Name, layer.Name)
			continue
		}
		byDir[layer.Dir] = layer
	}

	for i := range set.Resources {
		resource := &set.Resources[i]
		layer, ok := nearestLayer(byDir, relDir(filepath.Dir(resource.Position.File)))
		if !ok {
			errs.Add(resource.Position, "%s has no Layer document above it, so it belongs to no layer", resource.Ref())
			continue
		}
		resource.Layer = layer.Name
		resource.LayerDir = layer.Dir
	}
}

// nearestLayer walks up from dir until a directory holds a Layer document.
func nearestLayer(byDir map[string]document.Layer, dir string) (document.Layer, bool) {
	for {
		if layer, ok := byDir[dir]; ok {
			return layer, true
		}
		if dir == "" {
			return document.Layer{}, false
		}
		cut := strings.LastIndexByte(dir, '/')
		if cut < 0 {
			dir = ""
			continue
		}
		dir = dir[:cut]
	}
}
