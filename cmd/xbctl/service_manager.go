package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type initSystem string

const (
	initSystemd initSystem = "systemd"
	initOpenRC  initSystem = "openrc"
)

func detectInitSystem() (initSystem, error) {
	if commandAvailable("systemctl") && directoryExists("/run/systemd/system") {
		return initSystemd, nil
	}
	if commandAvailable("rc-service") && commandAvailable("rc-update") {
		return initOpenRC, nil
	}
	if commandAvailable("systemctl") && fileExists(systemdServiceFilePath) {
		return initSystemd, nil
	}
	return "", errors.New("no supported service manager found (systemd or OpenRC required)")
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func runManagedServiceAction(action string, extra ...string) error {
	manager, err := detectInitSystem()
	if err != nil {
		return err
	}

	var name string
	var args []string
	switch manager {
	case initSystemd:
		name = "systemctl"
		switch action {
		case "status":
			args = append([]string{"status", systemdServiceName, "--no-pager"}, extra...)
		case "reload-manager":
			args = []string{"daemon-reload"}
		case "start", "stop", "restart", "enable", "disable":
			args = append([]string{action, systemdServiceName}, extra...)
		default:
			return fmt.Errorf("unsupported service action: %s", action)
		}
	case initOpenRC:
		switch action {
		case "enable":
			name, args = "rc-update", append([]string{"add", serviceName, "default"}, extra...)
		case "disable":
			name, args = "rc-update", append([]string{"del", serviceName, "default"}, extra...)
		case "reload-manager":
			return nil
		case "status", "start", "stop", "restart":
			name, args = "rc-service", append([]string{serviceName, action}, extra...)
		default:
			return fmt.Errorf("unsupported service action: %s", action)
		}
	}
	return runPrivilegedCommand(name, args...)
}

func runManagedServiceLogs(args []string) error {
	manager, err := detectInitSystem()
	if err != nil {
		return err
	}
	if manager == initSystemd {
		if len(args) == 0 {
			args = []string{"-f"}
		}
		return runPrivilegedCommand("journalctl", append([]string{"-u", systemdServiceName}, args...)...)
	}
	if len(args) == 0 {
		args = []string{"-f"}
	}
	return runPrivilegedCommand("tail", append(args, openRCLogPath)...)
}

func managedServiceState() string {
	manager, err := detectInitSystem()
	if err != nil {
		return "unknown"
	}
	if manager == initSystemd {
		out, commandErr := exec.Command("systemctl", "is-active", systemdServiceName).CombinedOutput()
		state := strings.TrimSpace(string(out))
		if state != "" {
			return state
		}
		if commandErr != nil {
			return "inactive"
		}
		return "active"
	}
	if exec.Command("rc-service", "--quiet", serviceName, "status").Run() == nil {
		return "active"
	}
	return "inactive"
}

func waitForManagedServiceHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var state, health string
	for {
		state = managedServiceState()
		health = instanceAwareHealth()
		if state == "active" && (health == "ok" || health == "disabled") {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service=%s health=%s after %s", state, health, timeout)
		}
		time.Sleep(time.Second)
	}
}

func runPrivilegedCommand(name string, args ...string) error {
	if os.Geteuid() == 0 {
		return runCommand(name, args...)
	}
	if commandAvailable("sudo") {
		return runCommand("sudo", append([]string{name}, args...)...)
	}
	if commandAvailable("doas") {
		return runCommand("doas", append([]string{name}, args...)...)
	}
	return fmt.Errorf("%s requires root privileges; run coradectl as root", name)
}

func writeManagedServiceFile() error {
	manager, err := detectInitSystem()
	if err != nil {
		return err
	}
	if manager == initOpenRC {
		unit := fmt.Sprintf(`#!/sbin/openrc-run

name="Corade device platform Agent"
description="Corade device platform Agent"
supervisor="supervise-daemon"
command="%s"
command_args="-c %s"
directory="%s"
pidfile="/run/${RC_SVCNAME}.pid"
respawn_delay=5
respawn_max=0
output_log="%s"
error_log="%s"
required_files="%s"

if [ -r "%s" ]; then
  set -a
  . "%s"
  set +a
fi
if [ -r "%s" ]; then
  set -a
  . "%s"
  set +a
fi

depend() {
  need net
  after firewall
}
`, defaultBinaryPath, defaultConfigPath, defaultInstallRoot, openRCLogPath, openRCLogPath,
			defaultConfigPath, agentEnvironmentPath, agentEnvironmentPath, defaultCredentialsPath, defaultCredentialsPath)
		if err := os.MkdirAll(openRCLogDir, 0o750); err != nil {
			return err
		}
		if file, createErr := os.OpenFile(openRCLogPath, os.O_CREATE, 0o640); createErr != nil {
			return createErr
		} else {
			file.Close()
		}
		if err := os.Chmod(openRCLogPath, 0o640); err != nil {
			return err
		}
		if err := os.WriteFile(openRCServiceFilePath, []byte(unit), 0o755); err != nil {
			return err
		}
		return os.Chmod(openRCServiceFilePath, 0o755)
	}

	unit := fmt.Sprintf(`[Unit]
Description=Corade Node Backend
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
EnvironmentFile=-%s
EnvironmentFile=-%s
ExecStart=%s -c %s
Restart=always
RestartSec=5
LimitNOFILE=1048576
NoNewPrivileges=true
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, defaultInstallRoot, agentEnvironmentPath, defaultCredentialsPath, defaultBinaryPath, defaultConfigPath)
	return os.WriteFile(systemdServiceFilePath, []byte(unit), 0o644)
}
