package deviceplatform

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const agentUpgradeScript = "curl -fsSL https://raw.githubusercontent.com/ClaraCora/CPanelde/main/install.sh | /bin/bash -s -- upgrade"

func scheduleAgentUpgrade(ctx context.Context, taskID string) error {
	unitSuffix := sanitizeUnitSuffix(taskID)
	if unitSuffix == "" {
		return fmt.Errorf("invalid upgrade task id %q", taskID)
	}
	command := exec.CommandContext(ctx, "systemd-run",
		"--unit=corade-agent-upgrade-"+unitSuffix,
		"--description=Upgrade Corade Agent",
		"--property=Type=oneshot",
		"--collect",
		"--no-block",
		"/bin/bash", "-lc", agentUpgradeScript,
	)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("start transient systemd unit: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func sanitizeUnitSuffix(value string) string {
	var result strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			result.WriteRune(char)
		}
	}
	return result.String()
}
