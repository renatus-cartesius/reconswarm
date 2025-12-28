package control

import (
	"io"
	"time"
)

// Controller defines the interface for remote system control
type Controller interface {
	// Close closes the connection
	Close() error

	// Run executes a command on the remote host
	Run(command string) error

	// OpenFile opens a remote file for reading and/or writing.
	// Uses standard os flags: os.O_RDONLY, os.O_WRONLY, os.O_RDWR, os.O_CREATE, os.O_TRUNC, etc.
	// Caller must close the returned file.
	OpenFile(path string, flags int) (io.ReadWriteCloser, error)

	// GetInstanceName returns the instance name
	GetInstanceName() string

	// Sync copies a file or directory from remote host to local machine.
	// Automatically detects whether the path is a file or directory.
	Sync(remotePath, localPath string) error
}

// Config defines configuration for creating controllers
type Config struct {
	Host           string
	User           string
	PrivateKey     string // PEM-encoded private key content (preferred)
	PrivateKeyPath string // Path to private key file (deprecated, use PrivateKey)
	Timeout        time.Duration
	SSHTimeout     time.Duration
	InstanceName   string
}

// NewController creates a new controller based on the config
func NewController(config Config) (Controller, error) {
	// For now, only SSH is supported
	return NewSSH(SSHConfig(config))
}
