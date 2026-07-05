// SPDX-License-Identifier: Apache-2.0

package document

import "gopkg.in/yaml.v3"

// mapping wraps a YAML mapping node so the parser can look keys up and report
// unknown ones, which yaml.v3 will not do for a generic node.
type mapping struct {
	file string
	root *yaml.Node
	keys map[string]*yaml.Node
}

func newMapping(file string, node *yaml.Node) mapping {
	m := mapping{file: file, root: node, keys: map[string]*yaml.Node{}}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind == yaml.ScalarNode {
			m.keys[key.Value] = node.Content[i+1]
		}
	}
	return m
}

func (m mapping) at(node *yaml.Node) Position {
	return Position{File: m.file, Line: node.Line}
}

// node returns the value for a key, and false when it is absent, which is not the
// same as set to empty.
func (m mapping) node(key string) (*yaml.Node, bool) {
	value, ok := m.keys[key]
	return value, ok
}

func (m mapping) text(key string) (string, bool) {
	value, ok := m.keys[key]
	if !ok || value.Kind != yaml.ScalarNode {
		return "", false
	}
	return value.Value, true
}

// sequence returns a list field's items, erroring if it is not a list of scalars.
func (m mapping) sequence(key string, errs *Errors) []*yaml.Node {
	value, ok := m.keys[key]
	if !ok {
		return nil
	}
	if value.Kind != yaml.SequenceNode {
		errs.Add(m.at(value), "%s has to be a list", key)
		return nil
	}
	items := make([]*yaml.Node, 0, len(value.Content))
	for _, item := range value.Content {
		if item.Kind != yaml.ScalarNode {
			errs.Add(m.at(item), "%s entries have to be strings", key)
			continue
		}
		items = append(items, item)
	}
	return items
}

// reject reports keys outside the allowed set. A misspelled field has to be an
// error, or it would be configuration that silently does nothing.
func (m mapping) reject(errs *Errors, docType string, allowed ...string) {
	permitted := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		permitted[key] = true
	}
	for i := 0; i+1 < len(m.root.Content); i += 2 {
		key := m.root.Content[i]
		if key.Kind != yaml.ScalarNode || permitted[key.Value] {
			continue
		}
		errs.Add(m.at(key), "%s has no field %q", docType, key.Value)
	}
}
