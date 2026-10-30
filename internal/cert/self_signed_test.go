package cert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"testing"

	"github.com/ClaraCora/CPanelde/internal/config"
)

func TestSelfSignedNameGenerationPersistenceAndChange(t *testing.T) {
	ctx := context.Background()
	cfg := config.CertConfig{CertMode: "self", Domain: "edge-a.example.com", CertDir: t.TempDir()}
	manager := NewManager(cfg)
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	first := manager.TLSCert()
	if _, err := tls.X509KeyPair(first.CertPEM, first.KeyPEM); err != nil {
		t.Fatal(err)
	}
	if changed, err := manager.Reconfigure(ctx, cfg); err != nil || changed {
		t.Fatalf("unchanged config rotated certificate: changed=%v, err=%v", changed, err)
	}
	cfg.Domain = "edge-b.example.com"
	if changed, err := manager.Reconfigure(ctx, cfg); err != nil || !changed {
		t.Fatalf("changed name did not rotate certificate: changed=%v, err=%v", changed, err)
	}
	second := manager.TLSCert()
	pair, err := tls.X509KeyPair(second.CertPEM, second.KeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := certificate.VerifyHostname(cfg.Domain); err != nil {
		t.Fatal(err)
	}
	restarted := NewManager(cfg)
	if err := restarted.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if !pemEqual(second, restarted.TLSCert()) {
		t.Fatal("restart did not reuse the persisted certificate")
	}
}
