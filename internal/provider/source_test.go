// SPDX-License-Identifier: Apache-2.0

package provider_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dsgnr/datum/internal/provider"
)

func TestSourceResolvesAgainstTheDeclaringLayer(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "layers", "web", "files"))
	write(t, filepath.Join(root, "layers", "web", "files", "key.asc"), "signing key")

	req := provider.Request{RepoRoot: root, LayerDir: "layers/web"}
	data, err := req.ReadSource("files/key.asc")
	if err != nil {
		t.Fatalf("ReadSource: %v", err)
	}
	if string(data) != "signing key" {
		t.Fatalf("read %q", data)
	}
}

func TestSourceRefusesToEscapeTheRepository(t *testing.T) {
	outside := t.TempDir()
	write(t, filepath.Join(outside, "secret"), "not yours")

	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "layers", "web"))
	req := provider.Request{RepoRoot: root, LayerDir: "layers/web"}

	// Relative to the layer, so the traversal genuinely lands outside the checkout
	// rather than somewhere that happens to still be inside it.
	escape, err := filepath.Rel(filepath.Join(root, "layers", "web"), filepath.Join(outside, "secret"))
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}

	for _, rel := range []string{"../../../etc/shadow", escape} {
		if _, err := req.SourcePath(rel); err == nil {
			t.Errorf("SourcePath(%q) was allowed", rel)
		}
	}
}

// A link inside the fleet pointing out of it is the case a string comparison misses,
// because the declared path looks fine and only the resolved one does not.
func TestSourceRefusesALinkOutOfTheRepository(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privilege on windows")
	}
	outside := t.TempDir()
	write(t, filepath.Join(outside, "secret"), "not yours")

	root := t.TempDir()
	layer := filepath.Join(root, "layers", "web")
	mkdirAll(t, layer)
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(layer, "key.asc")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	req := provider.Request{RepoRoot: root, LayerDir: "layers/web"}
	if _, err := req.SourcePath("key.asc"); err == nil {
		t.Fatal("a link out of the repository was allowed")
	}
}

func TestDigestIsStableAndPrefixed(t *testing.T) {
	first := provider.Digest([]byte("abc"))
	if first != provider.Digest([]byte("abc")) {
		t.Fatal("digest is not stable")
	}
	if first == provider.Digest([]byte("abd")) {
		t.Fatal("different content digested the same")
	}
	const want = "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if first != want {
		t.Fatalf("digest = %s, want %s", first, want)
	}
}

func mkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// A secrets placeholder is left alone by resolution for the host to fill in. Nothing does
// that yet, so one reaching a provider has to say so rather than report a missing file
// whose name contains braces.
func TestAnUnresolvedPlaceholderIsNotTreatedAsAFilename(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "layers", "web"))
	req := provider.Request{RepoRoot: root, LayerDir: "layers/web"}

	_, err := req.ReadSource("{{ secrets.signing_key }}")
	if err == nil {
		t.Fatal("a placeholder was read as a path")
	}
	for _, want := range []string{"placeholder", "not implemented"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "no such file") {
		t.Errorf("the error blames a missing file: %v", err)
	}
}
