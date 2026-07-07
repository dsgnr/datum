// SPDX-License-Identifier: Apache-2.0

package document

import "fmt"

// targetIdentityField says where each type's target identity comes from. A type
// absent from the map uses its name, which is the common case.
var targetIdentityField = map[string]string{
	"File":       "path",
	"Directory":  "path",
	"Symlink":    "path",
	"Repository": "id",
}

// TargetIdentity returns what a resource manages on the host.
func TargetIdentity(typeName, name string, desired Value) (string, error) {
	field, fromDesired := targetIdentityField[typeName]
	if !fromDesired {
		return name, nil
	}
	value, ok := desired.Lookup(field)
	if !ok || value.Kind != KindScalar || value.Scalar == "" {
		return "", fmt.Errorf("%s needs desired.%s, which is its target identity", typeName, field)
	}
	return value.Scalar, nil
}
