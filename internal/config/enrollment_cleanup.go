package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var enrollmentFileMu sync.Mutex

// ForgetEnrollmentToken removes bootstrap secrets only after the private Agent
// identity has been persisted. Keep the pinned panel key for future restarts.
func (c *Config) ForgetEnrollmentToken() error {
	enrollmentFileMu.Lock()
	defer enrollmentFileMu.Unlock()
	if c.sourcePath != "" {
		data, err := os.ReadFile(c.sourcePath)
		if err != nil {
			return fmt.Errorf("read config for enrollment cleanup: %w", err)
		}
		var document yaml.Node
		if err := yaml.Unmarshal(data, &document); err != nil {
			return err
		}
		if len(document.Content) == 0 {
			return errors.New("empty config during enrollment cleanup")
		}
		root := document.Content[0]
		if instances := mappingValue(root, "instances"); instances != nil {
			for _, instance := range instances.Content {
				control := mappingValue(instance, "control")
				if control != nil {
					id := mappingValue(control, "machine_id")
					if id != nil && id.Value == c.Control.MachineID {
						c.cleanControlNode(instance)
					}
				}
			}
		} else {
			c.cleanControlNode(root)
		}
		encoded, err := yaml.Marshal(&document)
		if err != nil {
			return err
		}
		if err := replaceEnrollmentFile(c.sourcePath, encoded); err != nil {
			return err
		}
		// The shipped systemd/OpenRC installer stores the bootstrap value here.
		// Retain the file (and unrelated assignments) because service definitions
		// reference it, but remove the credential assignment before reconnecting.
		envPath := filepath.Join(filepath.Dir(c.sourcePath), "agent.env")
		envKey := c.Control.TokenEnv
		if envKey == "" {
			envKey = "CORADE_AGENT_TOKEN"
		}
		if envKey != "" {
			environment, err := os.ReadFile(envPath)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if err == nil {
				var kept []string
				for _, line := range strings.Split(string(environment), "\n") {
					assignment := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
					if !strings.HasPrefix(assignment, envKey+"=") {
						kept = append(kept, line)
					}
				}
				if err := replaceEnrollmentFile(envPath, []byte(strings.Join(kept, "\n"))); err != nil {
					return err
				}
			}
		}
	}
	if c.Control.TokenEnv != "" {
		_ = os.Unsetenv(c.Control.TokenEnv)
	}
	_ = os.Unsetenv("CORADE_AGENT_TOKEN")
	c.Control.Token, c.Control.TokenEnv = "", ""
	return nil
}

func (c *Config) cleanControlNode(parent *yaml.Node) {
	control := mappingValue(parent, "control")
	if control == nil {
		return
	}
	id := mappingValue(control, "machine_id")
	if id != nil && id.Value != c.Control.MachineID {
		return
	}
	content := control.Content[:0]
	for i := 0; i+1 < len(control.Content); i += 2 {
		if key := control.Content[i].Value; key != "token" && key != "token_env" {
			content = append(content, control.Content[i], control.Content[i+1])
		}
	}
	control.Content = content
	// AutoInstanceID historically includes the bootstrap token. Persist its
	// current value so deleting that token never moves a multi-instance private
	// identity to a different directory or creates duplicate IDs on restart.
	if c.InstanceID != "" {
		if id := mappingValue(parent, "id"); id != nil {
			id.Value = c.InstanceID
		} else {
			parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "id"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: c.InstanceID})
		}
	}
	if key := mappingValue(control, "panel_public_key"); key != nil {
		key.Value = c.Control.PanelPublicKey
	} else {
		control.Content = append(control.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "panel_public_key"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: c.Control.PanelPublicKey})
	}
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func replaceEnrollmentFile(path string, data []byte) error {
	// Reconnecting with a persisted identity runs cleanup again. Replacing an
	// unchanged config would trigger its watcher and restart the Agent forever.
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && bytes.Equal(current, data) {
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".enrollment-cleanup-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
