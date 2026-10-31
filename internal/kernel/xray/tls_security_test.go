package xray

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	xraytls "github.com/xtls/xray-core/transport/internet/tls"
)

func TestPinnedCARequiresMatchingServerName(t *testing.T) {
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(err)
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Ephemeral audit CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	must(err)
	ca, err = x509.ParseCertificate(caDER)
	must(err)
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(err)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"attacker.example"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, caKey)
	must(err)
	digest := sha256.Sum256(caDER)
	chain := [][]byte{leafDER, caDER}
	check := func(name string) bool {
		cfg := (&xraytls.Config{ServerName: name, PinnedPeerCertSha256: [][]byte{digest[:]}}).GetTLSConfig()
		return cfg.VerifyPeerCertificate(chain, nil) == nil
	}
	if check("") {
		t.Fatal("pinned CA accepted a certificate without a verification name")
	}
	if check("expected.example") {
		t.Fatal("pinned CA accepted a certificate for another name")
	}
	if !check("attacker.example") {
		t.Fatal("matching certificate unexpectedly rejected")
	}
	leafDigest := sha256.Sum256(leafDER)
	cfg := (&xraytls.Config{PinnedPeerCertSha256: [][]byte{leafDigest[:]}}).GetTLSConfig()
	if err := cfg.VerifyPeerCertificate(chain, nil); err != nil {
		t.Fatalf("explicit leaf pin rejected: %v", err)
	}
}
