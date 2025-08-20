package main

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNewVPNManager(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Suppress logs during tests
	
	// Mock SSH client (we'd need to implement a mock for real tests)
	sshClient := &SSHClient{
		host:     "test-host",
		username: "test-user", 
		password: "test-pass",
		logger:   logger,
	}

	manager := NewVPNManager(sshClient, logger)

	if manager == nil {
		t.Fatal("NewVPNManager returned nil")
	}

	if manager.configPath != "/opt/etc/xray/configs/05_routing.json" {
		t.Errorf("Expected default config path, got %s", manager.configPath)
	}
}

func TestSetConfigPath(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	
	sshClient := &SSHClient{
		host:     "test-host",
		username: "test-user",
		password: "test-pass", 
		logger:   logger,
	}

	manager := NewVPNManager(sshClient, logger)
	customPath := "/custom/path/config.json"
	
	manager.SetConfigPath(customPath)
	
	if manager.GetConfigPath() != customPath {
		t.Errorf("Expected config path %s, got %s", customPath, manager.GetConfigPath())
	}
}

func TestIsTargetRule(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	
	sshClient := &SSHClient{
		host:     "test-host",
		username: "test-user",
		password: "test-pass",
		logger:   logger,
	}

	manager := NewVPNManager(sshClient, logger)

	tests := []struct {
		name     string
		rule     Rule
		expected bool
	}{
		{
			name: "valid rule",
			rule: Rule{
				InboundTag: []string{"redirect", "tproxy"},
				Network:    "tcp,udp",
			},
			expected: true,
		},
		{
			name: "missing redirect",
			rule: Rule{
				InboundTag: []string{"tproxy"},
				Network:    "tcp,udp",
			},
			expected: false,
		},
		{
			name: "missing tproxy",
			rule: Rule{
				InboundTag: []string{"redirect"},
				Network:    "tcp,udp",
			},
			expected: false,
		},
		{
			name: "wrong network",
			rule: Rule{
				InboundTag: []string{"redirect", "tproxy"},
				Network:    "tcp",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.isTargetRule(tt.rule)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}