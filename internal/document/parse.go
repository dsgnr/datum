// SPDX-License-Identifier: Apache-2.0

package document

import (
	"bytes"
	"errors"
	"io"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Parsed is what one file contributed.
type Parsed struct {
	Fleets    []Fleet
	Hosts     []Host
	Layers    []Layer
	Resources []Resource
}

// ParseFile decodes every document in one file, returning what it read along with
// every problem it found.
func ParseFile(file string, data []byte) (Parsed, Errors) {
	var out Parsed
	var errs Errors

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var root yaml.Node
		err := decoder.Decode(&root)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			errs.Add(Position{File: file}, "cannot read YAML: %v", err)
			return out, errs
		}
		// A document separator with nothing after it decodes to an empty node.
		if root.Kind == 0 {
			continue
		}
		parseDocument(file, &root, &out, &errs)
	}
	return out, errs
}

func parseDocument(file string, root *yaml.Node, out *Parsed, errs *Errors) {
	body := root
	if body.Kind == yaml.DocumentNode {
		if len(body.Content) == 0 {
			return
		}
		body = body.Content[0]
	}
	pos := Position{File: file, Line: body.Line}
	if body.Kind != yaml.MappingNode {
		errs.Add(pos, "a document has to be a mapping")
		return
	}

	fields := newMapping(file, body)

	version, ok := fields.text("datum")
	if !ok {
		errs.Add(pos, "missing datum, which declares the schema version")
		return
	}
	if version != SchemaVersion {
		errs.Add(pos, "unsupported schema version %q, this agent supports %s", version, SchemaVersion)
		return
	}

	docType, ok := fields.text("type")
	if !ok {
		errs.Add(pos, "missing type")
		return
	}

	name, ok := fields.text("name")
	if !ok {
		errs.Add(pos, "missing name")
		return
	}
	if !ValidName(name) {
		errs.Add(pos, "name %q is not allowed, use letters, digits, dot, hyphen, underscore or plus", name)
		return
	}

	switch docType {
	case TypeFleet:
		out.Fleets = append(out.Fleets, parseFleet(name, pos, fields, errs))
	case TypeHost:
		out.Hosts = append(out.Hosts, parseHost(name, pos, fields, errs))
	case TypeLayer:
		out.Layers = append(out.Layers, parseLayer(name, pos, fields, errs))
	default:
		if !ResourceTypes[docType] {
			errs.Add(pos, "unrecognised type %q", docType)
			return
		}
		out.Resources = append(out.Resources, parseResource(docType, name, pos, fields, errs))
	}
}

func parseFleet(name string, pos Position, fields mapping, errs *Errors) Fleet {
	fleet := Fleet{Name: name, Position: pos}
	fields.reject(errs, "Fleet", "datum", "type", "name", "exclude")
	for _, node := range fields.sequence("exclude", errs) {
		fleet.Exclude = append(fleet.Exclude, node.Value)
	}
	return fleet
}

func parseHost(name string, pos Position, fields mapping, errs *Errors) Host {
	host := Host{Name: name, Position: pos, Labels: map[string]string{}}
	fields.reject(errs, "Host", "datum", "type", "name", "labels")

	labels, ok := fields.node("labels")
	if !ok {
		return host
	}
	if labels.Kind != yaml.MappingNode {
		errs.Add(Position{File: pos.File, Line: labels.Line}, "labels has to be a mapping")
		return host
	}
	for i := 0; i+1 < len(labels.Content); i += 2 {
		key, value := labels.Content[i], labels.Content[i+1]
		keyPos := Position{File: pos.File, Line: key.Line}
		switch {
		case !ValidLabelKey(key.Value):
			errs.Add(keyPos, "label key %q is not allowed", key.Value)
		case isReservedLabel(key.Value):
			errs.Add(keyPos, "label key %q is reserved, %s labels are injected during resolution", key.Value, ReservedLabelPrefix)
		case value.Kind != yaml.ScalarNode:
			errs.Add(keyPos, "label %q has to be a string", key.Value)
		case !ValidName(value.Value):
			errs.Add(keyPos, "label value %q is not allowed", value.Value)
		default:
			host.Labels[key.Value] = value.Value
		}
	}
	return host
}

func isReservedLabel(key string) bool {
	return len(key) > len(ReservedLabelPrefix) && key[:len(ReservedLabelPrefix)] == ReservedLabelPrefix
}

func parseLayer(name string, pos Position, fields mapping, errs *Errors) Layer {
	layer := Layer{Name: name, Position: pos}
	fields.reject(errs, "Layer", "datum", "type", "name", "precedence", "match")

	if node, ok := fields.node("precedence"); ok {
		value, err := strconv.Atoi(node.Value)
		switch {
		case node.Tag != "!!int":
			errs.Add(fields.at(node), "precedence has to be an integer")
		case err != nil:
			errs.Add(fields.at(node), "precedence %q is not an integer", node.Value)
		default:
			layer.Precedence = value
		}
	}

	if node, ok := fields.node("match"); ok {
		layer.Match = parseMatcher(pos.File, node, errs)
	}
	return layer
}

func parseMatcher(file string, node *yaml.Node, errs *Errors) Matcher {
	var m Matcher
	pos := Position{File: file, Line: node.Line}
	if node.Kind != yaml.MappingNode {
		errs.Add(pos, "match has to be a mapping")
		return m
	}
	inner := newMapping(file, node)
	inner.reject(errs, "match", "labels", "oneOf", "noneOf", "has", "missing")

	if labels, ok := inner.node("labels"); ok {
		m.Labels = readStringMap(file, labels, errs)
	}
	if oneOf, ok := inner.node("oneOf"); ok {
		m.OneOf = readStringListMap(file, oneOf, errs)
	}
	if noneOf, ok := inner.node("noneOf"); ok {
		m.NoneOf = readStringListMap(file, noneOf, errs)
	}
	for _, item := range inner.sequence("has", errs) {
		m.Has = append(m.Has, item.Value)
	}
	for _, item := range inner.sequence("missing", errs) {
		m.Missing = append(m.Missing, item.Value)
	}
	return m
}

func parseResource(docType, name string, pos Position, fields mapping, errs *Errors) Resource {
	resource := Resource{Type: docType, Name: name, Position: pos}
	fields.reject(errs, docType, "datum", "type", "name", "requires", "restartOn", "reloadOn", "desired")

	resource.Requires = readReferences(pos.File, fields, "requires", errs)
	resource.RestartOn = readReferences(pos.File, fields, "restartOn", errs)
	resource.ReloadOn = readReferences(pos.File, fields, "reloadOn", errs)

	if len(resource.RestartOn) > 0 && len(resource.ReloadOn) > 0 {
		errs.Add(pos, "restartOn and reloadOn cannot both be declared, they ask for different things on the same event")
	}

	desired, ok := fields.node("desired")
	if !ok {
		errs.Add(pos, "missing desired")
		return resource
	}
	resource.Desired = readValue(pos.File, desired, errs)
	return resource
}

func readReferences(file string, fields mapping, key string, errs *Errors) []Reference {
	var refs []Reference
	for _, item := range fields.sequence(key, errs) {
		ref, ok := ParseReference(item.Value)
		if !ok {
			errs.Add(Position{File: file, Line: item.Line}, "%s entry %q is not a resource reference, write it as Type[name]", key, item.Value)
			continue
		}
		refs = append(refs, ref)
	}
	return refs
}

// readValue turns a YAML node into a Value tree. Scalars keep whether they were
// quoted, so validation can tell "0640" from 0640.
func readValue(file string, node *yaml.Node, errs *Errors) Value {
	switch node.Kind {
	case yaml.ScalarNode:
		if err := SafeText(node.Value); err != nil {
			errs.Add(Position{File: file, Line: node.Line}, "value %s", err)
		}
		quoted := node.Style == yaml.SingleQuotedStyle || node.Style == yaml.DoubleQuotedStyle
		return Value{Kind: KindScalar, Scalar: node.Value, Quoted: quoted}
	case yaml.SequenceNode:
		out := Value{Kind: KindList}
		for _, item := range node.Content {
			out.List = append(out.List, readValue(file, item, errs))
		}
		return out
	case yaml.MappingNode:
		out := Value{Kind: KindMap, Map: map[string]Value{}}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Kind != yaml.ScalarNode {
				errs.Add(Position{File: file, Line: key.Line}, "a field name has to be a string")
				continue
			}
			if !ValidName(key.Value) {
				errs.Add(Position{File: file, Line: key.Line}, "field name %q is not allowed", key.Value)
				continue
			}
			out.Map[key.Value] = readValue(file, value, errs)
		}
		return out
	case yaml.AliasNode:
		errs.Add(Position{File: file, Line: node.Line}, "YAML anchors and aliases are not supported, write the value out")
		return Value{Kind: KindScalar}
	default:
		return Value{Kind: KindScalar}
	}
}

func readStringMap(file string, node *yaml.Node, errs *Errors) map[string]string {
	out := map[string]string{}
	if node.Kind != yaml.MappingNode {
		errs.Add(Position{File: file, Line: node.Line}, "expected a mapping of label names to values")
		return out
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if !ValidLabelKey(key.Value) {
			errs.Add(Position{File: file, Line: key.Line}, "label key %q is not allowed", key.Value)
			continue
		}
		if value.Kind != yaml.ScalarNode {
			errs.Add(Position{File: file, Line: key.Line}, "label %q has to be a string", key.Value)
			continue
		}
		out[key.Value] = value.Value
	}
	return out
}

func readStringListMap(file string, node *yaml.Node, errs *Errors) map[string][]string {
	out := map[string][]string{}
	if node.Kind != yaml.MappingNode {
		errs.Add(Position{File: file, Line: node.Line}, "expected a mapping of label names to lists of values")
		return out
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if !ValidLabelKey(key.Value) {
			errs.Add(Position{File: file, Line: key.Line}, "label key %q is not allowed", key.Value)
			continue
		}
		if value.Kind != yaml.SequenceNode {
			errs.Add(Position{File: file, Line: key.Line}, "label %q has to be a list of values", key.Value)
			continue
		}
		values := make([]string, 0, len(value.Content))
		for _, item := range value.Content {
			values = append(values, item.Value)
		}
		out[key.Value] = values
	}
	return out
}
