package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

// SSHClient represents a secure SSH client for router management
type SSHClient struct {
	host     string
	username string
	password string
	client   *ssh.Client
	logger   *logrus.Logger
}

// NewSSHClient creates a new SSH client instance
func NewSSHClient(host, username, password string, logger *logrus.Logger) (*SSHClient, error) {
	if host == "" || username == "" || password == "" {
		return nil, fmt.Errorf("SSH connection parameters cannot be empty")
	}

	return &SSHClient{
		host:     host,
		username: username,
		password: password,
		logger:   logger,
	}, nil
}

// Connect establishes SSH connection to the router
func (s *SSHClient) Connect() error {
	config := &ssh.ClientConfig{
		User: s.username,
		Auth: []ssh.AuthMethod{
			ssh.Password(s.password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Note: In production, use proper host key verification
		Timeout:         30 * time.Second,
	}

	// Add default SSH port if not specified
	host := s.host
	if !containsPort(host) {
		host += ":22"
	}

	client, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return fmt.Errorf("failed to connect to SSH server: %w", err)
	}

	s.client = client
	s.logger.WithField("host", s.host).Info("SSH connection established")
	return nil
}

// Disconnect closes the SSH connection
func (s *SSHClient) Disconnect() error {
	if s.client != nil {
		err := s.client.Close()
		s.client = nil
		s.logger.WithField("host", s.host).Info("SSH connection closed")
		return err
	}
	return nil
}

// ExecuteCommand executes a command on the remote server
func (s *SSHClient) ExecuteCommand(command string) (string, error) {
	if s.client == nil {
		if err := s.Connect(); err != nil {
			return "", fmt.Errorf("failed to establish SSH connection: %w", err)
		}
	}

	session, err := s.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	s.logger.WithField("command", command).Debug("Executing SSH command")

	output, err := session.CombinedOutput(command)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"command": command,
			"error":   err,
			"output":  string(output),
		}).Error("SSH command execution failed")
		return string(output), fmt.Errorf("command execution failed: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"command": command,
		"output":  string(output),
	}).Debug("SSH command executed successfully")

	return string(output), nil
}

// ReadFile reads a file from the remote server
func (s *SSHClient) ReadFile(filePath string) (string, error) {
	command := fmt.Sprintf("cat %s", filePath)
	return s.ExecuteCommand(command)
}

// WriteFile writes content to a file on the remote server
func (s *SSHClient) WriteFile(filePath, content string) error {
	// Create a backup first
	backupCommand := fmt.Sprintf("cp %s %s.backup.$(date +%%Y%%m%%d-%%H%%M%%S)", filePath, filePath)
	if _, err := s.ExecuteCommand(backupCommand); err != nil {
		s.logger.WithError(err).Warn("Failed to create backup, proceeding anyway")
	}

	// Write the new content
	command := fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", filePath, content)
	_, err := s.ExecuteCommand(command)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	s.logger.WithField("file", filePath).Info("File written successfully")
	return nil
}

// RestartService restarts Xray service using xkeen command
func (s *SSHClient) RestartService() error {
	command := "xkeen -restart"
	output, err := s.ExecuteCommand(command)
	if err != nil {
		return fmt.Errorf("failed to restart Xray service: %w (output: %s)", err, output)
	}

	s.logger.Info("Xray service restarted successfully using xkeen")
	return nil
}

// StartService starts Xray service using xkeen command
func (s *SSHClient) StartService() error {
	command := "export PATH=/opt/sbin:/opt/bin:/opt/usr/sbin:/opt/usr/bin:/usr/sbin:/usr/bin:/sbin:/bin && cd /opt/etc/xray/configs && xkeen -start"
	output, err := s.ExecuteCommand(command)
	if err != nil {
		return fmt.Errorf("failed to start Xray service: %w (output: %s)", err, output)
	}

	s.logger.Info("Xray service started successfully using xkeen")
	return nil
}

// StopService stops Xray service using xkeen command
func (s *SSHClient) StopService() error {
	command := "export PATH=/opt/sbin:/opt/bin:/opt/usr/sbin:/opt/usr/bin:/usr/sbin:/usr/bin:/sbin:/bin && cd /opt/etc/xray/configs && xkeen -stop"
	output, err := s.ExecuteCommand(command)
	if err != nil {
		return fmt.Errorf("failed to stop Xray service: %w (output: %s)", err, output)
	}

	s.logger.Info("Xray service stopped successfully using xkeen")
	return nil
}

// GetServiceStatus gets Xray service status using xkeen command
func (s *SSHClient) GetServiceStatus() (string, error) {
	command := "export PATH=/opt/sbin:/opt/bin:/opt/usr/sbin:/opt/usr/bin:/usr/sbin:/usr/bin:/sbin:/bin && cd /opt/etc/xray/configs && xkeen -status"
	s.logger.WithFields(logrus.Fields{
		"host":     s.host,
		"username": s.username,
		"command":  command,
	}).Info("Executing xkeen status command")
	
	output, err := s.ExecuteCommand(command)
	if err != nil {
		return "", fmt.Errorf("failed to get Xray service status: %w (output: %s)", err, output)
	}

	// Log raw output first
	s.logger.WithFields(logrus.Fields{
		"command":    command,
		"raw_output": output,
		"raw_bytes":  []byte(output),
		"raw_length": len(output),
	}).Info("Raw xkeen -status output")
	
	// Filter out the "ps: applet not found" error from xkeen output
	// Only keep lines that contain actual status information
	lines := strings.Split(output, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && 
		   !strings.Contains(line, "ps: applet not found") && 
		   !strings.Contains(line, "applet not found") {
			cleanLines = append(cleanLines, line)
		}
	}
	
	cleanOutput := strings.Join(cleanLines, "\n")
	s.logger.WithFields(logrus.Fields{
		"clean_output": cleanOutput,
		"clean_bytes":  []byte(cleanOutput),
		"clean_length": len(cleanOutput),
	}).Info("Cleaned xkeen -status output")
	return cleanOutput, nil
}


// CheckConnection verifies if the SSH connection is still active
func (s *SSHClient) CheckConnection() error {
	if s.client == nil {
		return fmt.Errorf("SSH client is not connected")
	}

	_, err := s.ExecuteCommand("echo 'connection_test'")
	return err
}

// containsPort checks if the host address already contains a port
func containsPort(host string) bool {
	return strings.Contains(host, ":")
}
