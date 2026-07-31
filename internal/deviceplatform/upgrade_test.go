package deviceplatform

import (
	"context"
	"strings"
	"testing"
)

func TestBuildUpgradeCommandSystemdUsesPOSIXShell(t *testing.T) {
	command := buildUpgradeCommand(context.Background(), upgradeSystemd, "task-1")
	joined := strings.Join(command.Args, " ")
	if !strings.Contains(joined, "systemd-run") || !strings.Contains(joined, "/bin/sh -c") {
		t.Fatalf("unexpected systemd upgrade command: %q", joined)
	}
	if strings.Contains(joined, "/bin/bash") {
		t.Fatalf("systemd command must not require bash: %q", joined)
	}
}

func TestBuildUpgradeCommandOpenRCIsDetachedAndLogged(t *testing.T) {
	command := buildUpgradeCommand(context.Background(), upgradeOpenRC, "task-1")
	joined := strings.Join(command.Args, " ")
	for _, expected := range []string{"nohup", "sleep 2", upgradeLogPath, "upgrade_script", "/bin/sh"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("OpenRC command %q does not contain %q", joined, expected)
		}
	}
}

func TestSanitizeUnitSuffix(t *testing.T) {
	if got := sanitizeUnitSuffix("task/../42 !"); got != "task42" {
		t.Fatalf("sanitizeUnitSuffix() = %q, want task42", got)
	}
}
