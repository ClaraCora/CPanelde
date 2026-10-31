package releaseverify

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenSSLSignatureInteroperability(t *testing.T) {
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("OpenSSL is unavailable; CI installs OpenSSL 3")
	}
	dir := t.TempDir()
	private, public := filepath.Join(dir, "private.pem"), filepath.Join(dir, "public.pem")
	manifestPath, signaturePath := filepath.Join(dir, "manifest"), filepath.Join(dir, "signature")
	data := []byte(fmt.Sprintf("%x  fixture\n", sha256.Sum256([]byte("cross-implementation fixture"))))
	if err := os.WriteFile(manifestPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"genpkey", "-algorithm", "ED25519", "-out", private},
		{"pkey", "-in", private, "-pubout", "-out", public},
		{"pkeyutl", "-sign", "-inkey", private, "-rawin", "-in", manifestPath, "-out", signaturePath},
	} {
		if out, err := exec.Command(openssl, args...).CombinedOutput(); err != nil {
			t.Fatalf("OpenSSL: %v: %s", err, out)
		}
	}
	publicPEM, err := os.ReadFile(public)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(publicPEM)
	if block == nil {
		t.Fatal("invalid OpenSSL PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := os.ReadFile(signaturePath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := verifyWithKey(data, signature, parsed.(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyReader("fixture", strings.NewReader("cross-implementation fixture")); err != nil {
		t.Fatal(err)
	}
}

func TestSignedManifestRejectsTamperingAndWrongPublisher(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	asset := "verified binary fixture"
	version := "v2.0.8+fixture\n"
	data := []byte(fmt.Sprintf("%x  corade-linux-amd64\n%x  agent-version.txt\n", sha256.Sum256([]byte(asset)), sha256.Sum256([]byte(version))))
	signature := ed25519.Sign(private, data)
	manifest, err := verifyWithKey(data, signature, public)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyReader("corade-linux-amd64", strings.NewReader(asset)); err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyReader("corade-linux-amd64", strings.NewReader(asset+"tamper")); err == nil {
		t.Fatal("tampered asset accepted")
	}
	if err := manifest.VerifyReader("coradectl-linux-amd64", strings.NewReader(asset)); err == nil {
		t.Fatal("unsigned asset name accepted")
	}
	changed := append([]byte(nil), data...)
	changed[0] ^= 1
	if _, err := verifyWithKey(changed, signature, public); err == nil {
		t.Fatal("tampered manifest accepted")
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := verifyWithKey(data, signature, other); err == nil {
		t.Fatal("untrusted signer accepted")
	}
	if _, err := Verify(data, signature); err == nil {
		t.Fatal("test signer accepted by production trust anchor")
	}
	path := filepath.Join(t.TempDir(), "agent-version.txt")
	if err := os.WriteFile(path, []byte(version), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"latest", "v2.0.8", "v2.0.8+fixture"} {
		if _, err := manifest.VerifyVersion(path, expected); err != nil {
			t.Fatal(err)
		}
	}
	for _, expected := range []string{"v2.0.7", "v2.0.8+another"} {
		if _, err := manifest.VerifyVersion(path, expected); err == nil {
			t.Fatalf("version mismatch %s accepted", expected)
		}
	}
	for _, name := range []string{"../binary", "/binary", "a/b", "asset asset"} {
		bad := []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(nil), name))
		if _, err := verifyWithKey(bad, ed25519.Sign(private, bad), public); err == nil {
			t.Fatalf("unsafe asset %q accepted", name)
		}
	}
	duplicate := append(append([]byte(nil), data...), data...)
	if _, err := verifyWithKey(duplicate, ed25519.Sign(private, duplicate), public); err == nil {
		t.Fatal("duplicate entries accepted")
	}
}
