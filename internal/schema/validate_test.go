// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/document"
)

// check validates a resource built from a flat map of fields and returns whatever
// was reported. Quoted marks the fields that were written with quotes in YAML,
// which matters for mode.
func check(typeName, name string, fields map[string]string, quoted ...string) document.Errors {
	desired := document.Value{Kind: document.KindMap, Map: map[string]document.Value{}}
	isQuoted := map[string]bool{}
	for _, q := range quoted {
		isQuoted[q] = true
	}
	for k, v := range fields {
		desired.Map[k] = document.Value{Kind: document.KindScalar, Scalar: v, Quoted: isQuoted[k]}
	}

	var errs document.Errors
	Validate(Resource{
		Ref:      document.Reference{Type: typeName, Name: name},
		Desired:  desired,
		Position: document.Position{File: "base/resources.yaml", Line: 1},
	}, &errs)
	return errs
}

func wantOK(t *testing.T, errs document.Errors) {
	t.Helper()
	if errs.Len() != 0 {
		t.Fatalf("unexpected errors:\n%s", errs.Error())
	}
}

func wantError(t *testing.T, errs document.Errors, want string) {
	t.Helper()
	if errs.Len() == 0 {
		t.Fatalf("expected an error mentioning %q, got none", want)
	}
	if !strings.Contains(errs.Error(), want) {
		t.Fatalf("expected an error mentioning %q, got:\n%s", want, errs.Error())
	}
}

// Every type the parser accepts needs a field table, or a valid document would be
// rejected for a reason that is Datum's fault.
func TestEveryResourceTypeHasAFieldTable(t *testing.T) {
	for typeName := range document.ResourceTypes {
		if !Known(typeName) {
			t.Errorf("%s is an accepted resource type with no field table", typeName)
		}
	}
	for typeName := range types {
		if !document.ResourceTypes[typeName] {
			t.Errorf("%s has a field table but is not an accepted resource type", typeName)
		}
	}
}

func TestValidFile(t *testing.T) {
	wantOK(t, check("File", "nginx-config", map[string]string{
		"path":   "/etc/nginx/nginx.conf",
		"owner":  "root",
		"group":  "root",
		"mode":   "0640",
		"source": "files/nginx.conf",
	}, "mode"))
}

func TestUnknownFieldIsRejected(t *testing.T) {
	errs := check("File", "nginx-config", map[string]string{
		"path":  "/etc/nginx/nginx.conf",
		"pathh": "/etc/nginx/other.conf",
	})
	wantError(t, errs, `has no field desired.pathh`)
}

func TestMissingRequiredFieldIsReported(t *testing.T) {
	wantError(t, check("File", "nginx-config", nil), "missing desired.path")
	wantError(t, check("Package", "nginx", nil), "missing desired.state")
	wantError(t, check("Sysctl", "net.ipv4.ip_forward", nil), "missing desired.value")
}

func TestPathHasToBeAbsolute(t *testing.T) {
	wantError(t, check("File", "f", map[string]string{"path": "etc/nginx.conf"}), "absolute path")
}

func TestPathRejectsTraversalAndEmptyComponents(t *testing.T) {
	wantError(t, check("File", "f", map[string]string{"path": "/etc/../etc/shadow"}), `contains ".."`)
	wantError(t, check("File", "f", map[string]string{"path": "/etc//nginx.conf"}), "empty path component")
	wantError(t, check("File", "f", map[string]string{"path": "/etc/./nginx.conf"}), `contains "."`)
}

func TestSourceHasToBeRelative(t *testing.T) {
	errs := check("File", "f", map[string]string{
		"path":   "/etc/nginx/nginx.conf",
		"source": "/etc/passwd",
	})
	wantError(t, errs, "relative to the layer directory")
}

func TestSourceCannotEscapeTheLayerDirectory(t *testing.T) {
	errs := check("File", "f", map[string]string{
		"path":   "/etc/nginx/nginx.conf",
		"source": "../../../etc/shadow",
	})
	wantError(t, errs, `contains ".."`)
}

func TestContentSourcesAreMutuallyExclusive(t *testing.T) {
	errs := check("File", "f", map[string]string{
		"path":    "/etc/nginx/nginx.conf",
		"content": "hello",
		"source":  "files/nginx.conf",
	})
	wantError(t, errs, "mutually exclusive")
}

// An unquoted mode is a number as far as YAML is concerned, so its base depends on
// the YAML version rather than on what the author meant.
func TestModeHasToBeQuoted(t *testing.T) {
	errs := check("File", "f", map[string]string{
		"path": "/etc/nginx/nginx.conf",
		"mode": "0640",
	})
	wantError(t, errs, "has to be quoted")
}

func TestModeHasToBeOctal(t *testing.T) {
	wantError(t, check("File", "f", map[string]string{
		"path": "/etc/nginx/nginx.conf", "mode": "0999",
	}, "mode"), "not octal")

	wantError(t, check("File", "f", map[string]string{
		"path": "/etc/nginx/nginx.conf", "mode": "64",
	}, "mode"), "three or four octal digits")
}

func TestSetuidNeedsAllowPrivileged(t *testing.T) {
	errs := check("File", "helper", map[string]string{
		"path": "/usr/local/bin/helper", "mode": "04755",
	}, "mode")
	wantError(t, errs, "three or four octal digits")

	errs = check("File", "helper", map[string]string{
		"path": "/usr/local/bin/helper", "mode": "4755",
	}, "mode")
	wantError(t, errs, "allowPrivileged")

	wantOK(t, check("File", "helper", map[string]string{
		"path": "/usr/local/bin/helper", "mode": "4755", "allowPrivileged": "true",
	}, "mode"))
}

func TestWorldWritableUnderASystemDirectoryNeedsAllowPrivileged(t *testing.T) {
	errs := check("File", "f", map[string]string{
		"path": "/etc/app/thing.conf", "mode": "0666",
	}, "mode")
	wantError(t, errs, "world writable")

	// Outside a system directory it is untidy rather than an escalation.
	wantOK(t, check("File", "f", map[string]string{
		"path": "/srv/app/thing.conf", "mode": "0666",
	}, "mode"))
}

func TestBooleanFields(t *testing.T) {
	wantError(t, check("File", "f", map[string]string{
		"path": "/etc/f", "sensitive": "yes",
	}), "true or false")

	wantOK(t, check("File", "f", map[string]string{
		"path": "/etc/f", "sensitive": "true",
	}))
}

func TestEnumFields(t *testing.T) {
	wantError(t, check("Service", "nginx", map[string]string{
		"state": "started", "enabled": "true",
	}), "not one of running, stopped")

	wantOK(t, check("Service", "nginx", map[string]string{
		"state": "running", "enabled": "true",
	}))
}

func TestIntegerFields(t *testing.T) {
	wantError(t, check("User", "deploy", map[string]string{
		"state": "present", "uid": "not-a-number",
	}), "has to be an integer")

	wantOK(t, check("User", "deploy", map[string]string{
		"state": "present", "uid": "1001",
	}))
}

func TestNameFieldsRejectDangerousText(t *testing.T) {
	wantError(t, check("File", "f", map[string]string{
		"path": "/etc/f", "owner": "root; rm -rf /",
	}), "is not allowed")
}

func TestListFields(t *testing.T) {
	desired := document.Value{Kind: document.KindMap, Map: map[string]document.Value{
		"state":  document.Scalar("present"),
		"groups": {Kind: document.KindList, List: []document.Value{document.Scalar("docker"), document.Scalar("bad name")}},
	}}
	var errs document.Errors
	Validate(Resource{Ref: document.Reference{Type: "User", Name: "deploy"}, Desired: desired}, &errs)
	wantError(t, errs, "is not a valid name")
}

func TestListFieldRejectsAScalar(t *testing.T) {
	wantError(t, check("User", "deploy", map[string]string{
		"state": "present", "groups": "docker",
	}), "has to be a list")
}

func TestSymlinkNeedsATargetWhenPresent(t *testing.T) {
	wantError(t, check("Symlink", "link", map[string]string{
		"path": "/etc/nginx/sites-enabled/app.conf",
	}), "missing desired.target")

	wantOK(t, check("Symlink", "link", map[string]string{
		"path":   "/etc/nginx/sites-enabled/app.conf",
		"target": "/etc/nginx/sites-available/app.conf",
	}))

	// A link declared absent has nothing to point at.
	wantOK(t, check("Symlink", "link", map[string]string{
		"path":  "/etc/nginx/sites-enabled/app.conf",
		"state": "absent",
	}))
}

// A package source that trusts no named key is root execution arranged through a
// configuration file, so saying so has to be explicit.
func TestRepositoryNeedsAKeyOrAnExplicitOptOut(t *testing.T) {
	wantError(t, check("Repository", "internal", map[string]string{
		"id": "internal", "url": "https://packages.internal/debian",
	}), "needs desired.unsigned: true")

	wantOK(t, check("Repository", "internal", map[string]string{
		"id": "internal", "url": "https://packages.internal/debian", "signingKey": "files/internal.asc",
	}))

	wantOK(t, check("Repository", "internal", map[string]string{
		"id": "internal", "url": "https://packages.internal/debian", "unsigned": "true",
	}))

	wantError(t, check("Repository", "internal", map[string]string{
		"id": "internal", "url": "https://packages.internal/debian",
		"signingKey": "files/internal.asc", "unsigned": "true",
	}), "contradict each other")
}

func TestRepositoryNeedsAUrlWhenPresent(t *testing.T) {
	wantError(t, check("Repository", "internal", map[string]string{
		"id": "internal", "unsigned": "true",
	}), "missing desired.url")

	wantOK(t, check("Repository", "internal", map[string]string{
		"id": "internal", "state": "absent",
	}))
}

func TestAbsentFileCannotAlsoDeclareContent(t *testing.T) {
	wantError(t, check("File", "f", map[string]string{
		"path": "/etc/f", "state": "absent", "content": "hello",
	}), "absent but also sets desired.content")
}

func TestDesiredHasToBeAMapping(t *testing.T) {
	var errs document.Errors
	Validate(Resource{
		Ref:     document.Reference{Type: "Package", Name: "nginx"},
		Desired: document.Scalar("present"),
	}, &errs)
	wantError(t, errs, "has to be a mapping of fields")
}
