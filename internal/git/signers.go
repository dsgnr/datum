// SPDX-License-Identifier: Apache-2.0

package git

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"strings"
)

// Signer is one entry in the allowed-signers file.
type Signer struct {
	// KeyID is the fingerprint, in the form ssh tooling prints it.
	KeyID string
	// Principals are the identities the entry applies to.
	Principals string
}

// Signers reads the keys a host trusts.
//
// The result is what the host is configured to trust, which is the question key
// rotation asks. It is unrelated to what signed the last revision, and a host that fell
// back to an older revision still reports its configured keys.
//
// A file that is not in allowed-signers format yields no keys and no error, since a
// fleet signing with gpg keeps its trust root in a keyring this cannot read.
func Signers(path string) ([]Signer, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var out []Signer
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if signer, ok := parseSigner(line); ok {
			out = append(out, signer)
		}
	}
	return out, scanner.Err()
}

// parseSigner reads `principals [options] keytype base64key [comment]`.
//
// Options are comma-separated and may appear between the principals and the key, so the
// key is found by looking for the first field that is a key type, not by position.
func parseSigner(line string) (Signer, bool) {
	fields := strings.Fields(line)
	for i := 1; i+1 < len(fields); i++ {
		if !strings.HasPrefix(fields[i], "ssh-") && !strings.HasPrefix(fields[i], "sk-") &&
			!strings.HasPrefix(fields[i], "ecdsa-") {
			continue
		}
		blob, err := base64.StdEncoding.DecodeString(fields[i+1])
		if err != nil {
			return Signer{}, false
		}
		return Signer{KeyID: fingerprint(blob), Principals: fields[0]}, true
	}
	return Signer{}, false
}

// fingerprint is the SHA256 form ssh-keygen -l prints, which is unpadded base64 of the
// digest of the raw key blob.
func fingerprint(blob []byte) string {
	sum := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}
