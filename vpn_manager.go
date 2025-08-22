package main

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
)

// VPNStatus represents the current VPN routing status
type VPNStatus string

const (
	VPNStatusEnabled  VPNStatus = "enabled"
	VPNStatusDisabled VPNStatus = "disabled"
	VPNStatusUnknown  VPNStatus = "unknown"
)

// VPNManager manages VPN routing configuration on Xkeen router
type VPNManager struct {
	sshClient  *SSHClient
	logger     *logrus.Logger
	configPath string
}

// XrayConfig represents the structure of Xray routing configuration
type XrayConfig struct {
	Routing *RoutingConfig `json:"routing,omitempty"`
}

// RoutingConfig represents the routing configuration
type RoutingConfig struct {
	DomainStrategy string `json:"domainStrategy,omitempty"`
	Rules          []Rule `json:"rules,omitempty"`
}

// Rule represents a routing rule
type Rule struct {
	Type        string      `json:"type,omitempty"`
	InboundTag  []string    `json:"inboundTag,omitempty"`
	OutboundTag string      `json:"outboundTag,omitempty"`
	Network     string      `json:"network,omitempty"`
	Domain      interface{} `json:"domain,omitempty"`
	IP          interface{} `json:"ip,omitempty"`
	Port        string      `json:"port,omitempty"`
	Protocol    interface{} `json:"protocol,omitempty"`
}

// NewVPNManager creates a new VPN manager instance
func NewVPNManager(sshClient *SSHClient, logger *logrus.Logger) *VPNManager {
	return &VPNManager{
		sshClient:  sshClient,
		logger:     logger,
		configPath: "/opt/etc/xray/configs/05_routing.json",
	}
}

// GetStatus retrieves the current VPN routing status
func (vm *VPNManager) GetStatus() (VPNStatus, error) {
	vm.logger.Debug("Getting VPN status")

	// Read the configuration file
	configContent, err := vm.sshClient.ReadFile(vm.configPath)
	if err != nil {
		return VPNStatusUnknown, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse the configuration
	var config XrayConfig
	if err := json.Unmarshal([]byte(configContent), &config); err != nil {
		return VPNStatusUnknown, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	// Find the routing rule we're interested in
	if config.Routing == nil {
		return VPNStatusUnknown, fmt.Errorf("no routing configuration found")
	}

	for _, rule := range config.Routing.Rules {
		if vm.isTargetRule(rule) {
			switch rule.OutboundTag {
			case "vless-reality":
				vm.logger.Debug("VPN status: enabled")
				return VPNStatusEnabled, nil
			case "direct":
				vm.logger.Debug("VPN status: disabled")
				return VPNStatusDisabled, nil
			default:
				vm.logger.WithField("outbound_tag", rule.OutboundTag).Warn("Unknown outbound tag")
				return VPNStatusUnknown, nil
			}
		}
	}

	return VPNStatusUnknown, fmt.Errorf("target routing rule not found")
}

// EnableVPN switches routing to use VPN (vless-reality outbound)
func (vm *VPNManager) EnableVPN() error {
	vm.logger.Info("Enabling VPN routing")
	return vm.setOutboundTag("vless-reality")
}

// DisableVPN switches routing to direct connection
func (vm *VPNManager) DisableVPN() error {
	vm.logger.Info("Disabling VPN routing")
	return vm.setOutboundTag("direct")
}

// setOutboundTag changes the outbound tag for the target routing rule
func (vm *VPNManager) setOutboundTag(outboundTag string) error {
	// Read current configuration
	configContent, err := vm.sshClient.ReadFile(vm.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse the configuration
	var config XrayConfig
	if err := json.Unmarshal([]byte(configContent), &config); err != nil {
		return fmt.Errorf("failed to parse config JSON: %w", err)
	}

	// Ensure routing configuration exists
	if config.Routing == nil {
		return fmt.Errorf("no routing configuration found")
	}

	// The default routing rule is always the last rule in the list
	if len(config.Routing.Rules) == 0 {
		return fmt.Errorf("no routing rules found")
	}
	
	lastRuleIndex := len(config.Routing.Rules) - 1
	lastRule := config.Routing.Rules[lastRuleIndex]
	
	// Verify this is indeed the default routing rule
	if !vm.isTargetRule(lastRule) {
		return fmt.Errorf("last rule is not the expected default routing rule")
	}
	
	vm.logger.WithFields(logrus.Fields{
		"rule_index":   lastRuleIndex,
		"old_outbound": lastRule.OutboundTag,
		"new_outbound": outboundTag,
	}).Info("Updating default routing rule")

	config.Routing.Rules[lastRuleIndex].OutboundTag = outboundTag

	// Marshal the updated configuration
	updatedContent, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}

	// Write the updated configuration back to the file
	if err := vm.sshClient.WriteFile(vm.configPath, string(updatedContent)); err != nil {
		return fmt.Errorf("failed to write updated config: %w", err)
	}

	// Restart Xray service to apply changes
	if err := vm.restartXrayService(); err != nil {
		vm.logger.WithError(err).Warn("Failed to restart Xray service, changes may not be applied immediately")
		// Don't return error here as the config was successfully updated
	}

	vm.logger.WithField("outbound_tag", outboundTag).Info("VPN routing configuration updated successfully")
	return nil
}

// isTargetRule checks if a rule is the target rule we want to modify
// This should be the default routing rule with inboundTag [redirect, tproxy] and network tcp,udp
func (vm *VPNManager) isTargetRule(rule Rule) bool {
	// Check if the rule has the correct inbound tags
	hasRedirect := false
	hasTproxy := false

	for _, tag := range rule.InboundTag {
		if tag == "redirect" {
			hasRedirect = true
		} else if tag == "tproxy" {
			hasTproxy = true
		}
	}

	// Must have both redirect and tproxy, and network must be exactly "tcp,udp"
	hasCorrectNetwork := rule.Network == "tcp,udp"

	return hasRedirect && hasTproxy && hasCorrectNetwork
}

// restartXrayService restarts the Xray service using xkeen
func (vm *VPNManager) restartXrayService() error {
	vm.logger.Info("Restarting Xray service using xkeen")

	// Use xkeen command to restart
	err := vm.sshClient.RestartService()
	if err != nil {
		vm.logger.WithError(err).Error("Failed to restart Xray service with xkeen")
		return err
	}

	vm.logger.Info("Successfully restarted Xray service")
	return nil
}

// ValidateConfiguration checks if the Xray configuration is valid
func (vm *VPNManager) ValidateConfiguration() error {
	vm.logger.Debug("Validating Xray configuration")

	// Read the configuration file
	configContent, err := vm.sshClient.ReadFile(vm.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Try to parse the JSON
	var config XrayConfig
	if err := json.Unmarshal([]byte(configContent), &config); err != nil {
		return fmt.Errorf("invalid JSON configuration: %w", err)
	}

	// Check if routing configuration exists
	if config.Routing == nil {
		return fmt.Errorf("no routing configuration found")
	}

	// Check if target rule exists
	targetRuleFound := false
	for _, rule := range config.Routing.Rules {
		if vm.isTargetRule(rule) {
			targetRuleFound = true
			break
		}
	}

	if !targetRuleFound {
		return fmt.Errorf("target routing rule not found")
	}

	vm.logger.Debug("Configuration validation passed")
	return nil
}

// GetConfigPath returns the path to the Xray configuration file
func (vm *VPNManager) GetConfigPath() string {
	return vm.configPath
}

// SetConfigPath sets a custom path to the Xray configuration file
func (vm *VPNManager) SetConfigPath(path string) {
	vm.configPath = path
	vm.logger.WithField("config_path", path).Info("Configuration path updated")
}

// StartVPNService starts the VPN service using xkeen command
func (vm *VPNManager) StartVPNService() error {
	vm.logger.Info("Starting VPN service using xkeen")
	return vm.sshClient.StartService()
}

// StopVPNService stops the VPN service using xkeen command
func (vm *VPNManager) StopVPNService() error {
	vm.logger.Info("Stopping VPN service using xkeen")
	return vm.sshClient.StopService()
}

// GetVPNServiceStatus gets the current VPN service status using xkeen command
func (vm *VPNManager) GetVPNServiceStatus() (string, error) {
	vm.logger.Debug("Getting VPN service status using xkeen")
	return vm.sshClient.GetServiceStatus()
}

