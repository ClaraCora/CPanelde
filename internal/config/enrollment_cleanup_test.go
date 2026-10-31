package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnrollmentCleanupPreservesMultiInstanceIdentityDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	data := "instances:\n"
	for i := 1; i <= 2; i++ {
		data += fmt.Sprintf("  - control:\n      mode: device-platform\n      url: https://panel.example\n      machine_id: mch_%d\n      token: fixture-machine-%d-bootstrap-credential\n      panel_public_key: fixture-panel-pin\n    health_port: 0\n", i, i)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := LoadRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	instances, err := root.NormalizeInstances()
	if err != nil {
		t.Fatal(err)
	}
	ids, dirs := make([]string, 2), make([]string, 2)
	for i, instance := range instances {
		ids[i], dirs[i] = instance.InstanceID, instance.Kernel.ConfigDir
		if err := instance.ForgetEnrollmentToken(); err != nil {
			t.Fatal(err)
		}
	}
	reloaded, err := LoadRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, instance := range reloaded.Instances {
		if instance.InstanceID != ids[i] || instance.Kernel.ConfigDir != dirs[i] || instance.Control.Token != "" {
			t.Fatalf("instance %d moved identity directory or retained credential", i)
		}
	}
}

func TestEnrollmentCleanupRemovesOnlyBootstrapSecretAndSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	secret := "bootstrap-secret-that-must-disappear"
	data := "control:\n  mode: device-platform\n  url: https://panel.example\n  machine_id: mch_test\n  token: " + secret + "\n  token_env: CORADE_AGENT_TOKEN\nhealth_port: 65530\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.env"), []byte("CORADE_AGENT_TOKEN="+secret+"\nUNRELATED=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Config{InstanceID: "fixture-instance", sourcePath: path, Control: ControlConfig{MachineID: "mch_test", Token: secret, TokenEnv: "CORADE_AGENT_TOKEN", PanelPublicKey: "pinned-panel-key"}}
	if err := c.ForgetEnrollmentToken(); err != nil {
		t.Fatal(err)
	}
	beforeConfig, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeEnv, err := os.Stat(filepath.Join(dir, "agent.env"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ForgetEnrollmentToken(); err != nil {
		t.Fatalf("cleanup not idempotent: %v", err)
	}
	// A newly loaded instance must also leave already-clean files in place;
	// otherwise each signed reconnect triggers another config watcher reload.
	reloaded, err := LoadRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	instances, err := reloaded.NormalizeInstances()
	if err != nil {
		t.Fatal(err)
	}
	if err := instances[0].ForgetEnrollmentToken(); err != nil {
		t.Fatal(err)
	}
	for name, before := range map[string]os.FileInfo{path: beforeConfig, filepath.Join(dir, "agent.env"): beforeEnv} {
		after, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
			t.Fatalf("repeated cleanup replaced or rewrote %s", name)
		}
	}
	for _, name := range []string{path, filepath.Join(dir, "agent.env")} {
		content, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), secret) || strings.Contains(string(content), "token_env:") {
			t.Fatalf("bootstrap secret/reference remains in %s", name)
		}
	}
	configData, _ := os.ReadFile(path)
	if !strings.Contains(string(configData), "pinned-panel-key") {
		t.Fatal("pinned key lost")
	}
	envData, _ := os.ReadFile(filepath.Join(dir, "agent.env"))
	if !strings.Contains(string(envData), "UNRELATED=value") {
		t.Fatal("unrelated environment removed")
	}
	if c.Control.Token != "" || c.Control.TokenEnv != "" {
		t.Fatal("in-memory bootstrap credential remains")
	}
}
