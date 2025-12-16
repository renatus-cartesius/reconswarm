package control

import (
	"os"
	"time"
)

// Controller defines the interface for remote system control
type Controller interface {
	// Close closes the connection
	Close() error

	// Run executes a command on the remote host
	Run(command string) error

	// ReadFile reads a file from the remote host
	ReadFile(remotePath string) (string, error)

	// WriteFile writes content to a file on the remote host
	WriteFile(remotePath, content string, mode os.FileMode) error

	// GetInstanceName returns the instance name
	GetInstanceName() string

	// Sync copies a file or directory from remote host to local machine using SFTP.
	// Automatically detects whether the path is a file or directory and handles accordingly.
	Sync(remotePath, localPath string) error
}

// Config defines configuration for creating controllers
type Config struct {
	Host           string
	User           string
	PrivateKey     string        // PEM-encoded private key content (preferred)
	PrivateKeyPath string        // Path to private key file (deprecated, use PrivateKey)
	Timeout        time.Duration
	SSHTimeout     time.Duration
	InstanceName   string
}

// NewController creates a new controller based on the config
func NewController(config Config) (Controller, error) {
	// For now, only SSH is supported
	sshConfig := SSHConfig{
		Host:           config.Host,
		User:           config.User,
		PrivateKey:     config.PrivateKey,
		PrivateKeyPath: config.PrivateKeyPath,
		Timeout:        config.Timeout,
		SSHTimeout:     config.SSHTimeout,
		InstanceName:   config.InstanceName,
	}

	return NewSSH(sshConfig)
}
