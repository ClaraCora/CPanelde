package singbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sagernet/sing-box/adapter"
	singLog "github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
)

// Reload does not replace the outbound manager's default tag. An explicit
// catch-all rule applies the desired final outbound in that same hot reload.
func reloadConfigBytes(cfg M) ([]byte, error) {
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var normalized map[string]any
	if err = json.Unmarshal(encoded, &normalized); err != nil {
		return nil, err
	}
	if route, ok := normalized["route"].(map[string]any); ok {
		if final, ok := route["final"].(string); ok && final != "" {
			rules, _ := route["rules"].([]any)
			route["rules"] = append(rules, map[string]any{"outbound": final})
		}
	}
	return json.Marshal(normalized)
}

func orderedOutbounds(outbounds []option.Outbound) ([]option.Outbound, error) {
	byTag := make(map[string]option.Outbound, len(outbounds))
	for _, outbound := range outbounds {
		byTag[outbound.Tag] = outbound
	}
	state := make(map[string]int, len(outbounds))
	result := make([]option.Outbound, 0, len(outbounds))
	var visit func(string) error
	visit = func(tag string) error {
		if state[tag] == 2 {
			return nil
		}
		if state[tag] == 1 {
			return fmt.Errorf("cyclic outbound dependency")
		}
		outbound, exists := byTag[tag]
		if !exists {
			return nil
		}
		state[tag] = 1
		raw, err := json.Marshal(outbound.Options)
		if err != nil {
			return err
		}
		var settings struct {
			Detour    string   `json:"detour"`
			Outbounds []string `json:"outbounds"`
		}
		if err = json.Unmarshal(raw, &settings); err != nil {
			return err
		}
		for _, dependency := range append(settings.Outbounds, settings.Detour) {
			if dependency != "" {
				if err = visit(dependency); err != nil {
					return err
				}
			}
		}
		state[tag] = 2
		result = append(result, outbound)
		return nil
	}
	for _, outbound := range outbounds {
		if err := visit(outbound.Tag); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// Outbounds must exist before rule construction. Create builds/starts a new
// outbound before replacing an existing tag. Dependencies are created first.
func replaceOutbounds(ctx context.Context, manager adapter.OutboundManager, router adapter.Router, desired, previous []option.Outbound) error {
	ordered, err := orderedOutbounds(desired)
	if err != nil {
		return err
	}
	previousByTag := make(map[string]string, len(previous))
	for _, outbound := range previous {
		data, _ := json.Marshal([]any{outbound.Type, outbound.Tag, outbound.Options})
		previousByTag[outbound.Tag] = string(data)
	}
	factory := singLog.NewNOPFactory()
	for _, outbound := range ordered {
		data, err := json.Marshal([]any{outbound.Type, outbound.Tag, outbound.Options})
		if err != nil {
			return err
		}
		if _, exists := manager.Outbound(outbound.Tag); exists && previousByTag[outbound.Tag] == string(data) {
			continue
		}
		logger := factory.NewLogger(fmt.Sprintf("outbound/%s[%s]", outbound.Type, outbound.Tag))
		if err = manager.Create(ctx, router, logger, outbound.Tag, outbound.Type, outbound.Options); err != nil {
			return fmt.Errorf("reload outbound %s: %w", outbound.Tag, err)
		}
	}
	return nil
}

func inboundFingerprint(inbounds []option.Inbound) []byte {
	values := make([]any, 0, len(inbounds))
	for _, inbound := range inbounds {
		values = append(values, []any{inbound.Type, inbound.Tag, inbound.Options})
	}
	encoded, _ := json.Marshal(values)
	return encoded
}

func removeObsoleteOutbounds(manager adapter.OutboundManager, previous, desired []option.Outbound) error {
	wanted := make(map[string]bool, len(desired))
	for _, outbound := range desired {
		wanted[outbound.Tag] = true
	}
	ordered, err := orderedOutbounds(previous)
	if err != nil {
		return err
	}
	// Dependents must be removed before their upstreams.
	for index := len(ordered) - 1; index >= 0; index-- {
		if tag := ordered[index].Tag; !wanted[tag] {
			if _, exists := manager.Outbound(tag); !exists {
				continue
			}
			if err = manager.Remove(tag); err != nil {
				return fmt.Errorf("remove outbound %s: %w", tag, err)
			}
		}
	}
	return nil
}
