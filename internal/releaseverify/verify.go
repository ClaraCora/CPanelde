// Package releaseverify authenticates release contents with an offline Ed25519
// trust anchor. Checksums fetched from the release alone are not a trust anchor.
package releaseverify

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed release-public-key.pem
var publicKeyPEM []byte

var assetName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
var releaseVersion = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:\+[a-zA-Z0-9._-]+)?$`)

type Manifest map[string][sha256.Size]byte

func Verify(manifest, signature []byte) (Manifest, error) {
	block, _ := pem.Decode(publicKeyPEM)
	if block == nil {
		return nil, errors.New("release public key is unavailable")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	public, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("release key is not Ed25519")
	}
	return verifyWithKey(manifest, signature, public)
}

func verifyWithKey(data, signature []byte, key ed25519.PublicKey) (Manifest, error) {
	if len(data) == 0 || len(data) > 64<<10 || len(signature) != ed25519.SignatureSize || !ed25519.Verify(key, data, signature) {
		return nil, errors.New("release manifest signature verification failed")
	}
	manifest := make(Manifest)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 || !assetName.MatchString(fields[1]) {
			return nil, errors.New("invalid signed release manifest entry")
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil {
			return nil, errors.New("invalid release digest")
		}
		if _, duplicate := manifest[fields[1]]; duplicate {
			return nil, errors.New("duplicate release asset")
		}
		var digest [sha256.Size]byte
		copy(digest[:], decoded)
		manifest[fields[1]] = digest
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(manifest) == 0 {
		return nil, errors.New("empty release manifest")
	}
	return manifest, nil
}

func (m Manifest) VerifyReader(name string, reader io.Reader) error {
	expected, ok := m[name]
	if !ok || !assetName.MatchString(name) {
		return errors.New("asset is absent from signed release manifest")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return err
	}
	if !bytes.Equal(hash.Sum(nil), expected[:]) {
		return fmt.Errorf("signed SHA-256 mismatch for %s", name)
	}
	return nil
}

func (m Manifest) VerifyFile(name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return m.VerifyReader(name, f)
}

func (m Manifest) VerifyVersion(versionFile, expected string) (string, error) {
	data, err := readBounded(versionFile, 256)
	if err != nil {
		return "", err
	}
	if err := m.VerifyReader("agent-version.txt", bytes.NewReader(data)); err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(data))
	if !releaseVersion.MatchString(version) {
		return "", errors.New("invalid signed release version")
	}
	expected = strings.TrimPrefix(expected, "corade-")
	if expected != "" && expected != "latest" && expected != version {
		if strings.Contains(expected, "+") || strings.SplitN(version, "+", 2)[0] != expected {
			return "", errors.New("signed release does not match requested version")
		}
	}
	return version, nil
}

func VerifyArtifact(directory, name, path, expectedVersion string) error {
	data, err := readBounded(filepath.Join(directory, "release-manifest.sha256"), 64<<10)
	if err != nil {
		return err
	}
	signature, err := readBounded(filepath.Join(directory, "release-manifest.sha256.sig"), ed25519.SignatureSize)
	if err != nil {
		return err
	}
	manifest, err := Verify(data, signature)
	if err != nil {
		return err
	}
	if _, err := manifest.VerifyVersion(filepath.Join(directory, "agent-version.txt"), expectedVersion); err != nil {
		return err
	}
	return manifest.VerifyFile(name, path)
}

func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("release metadata exceeds size limit")
	}
	return data, nil
}
