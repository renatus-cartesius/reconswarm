package control

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reconswarm/internal/logging"

	"github.com/pkg/sftp"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// SSH represents an SSH connection and provides methods for remote operations
type SSH struct {
	client       *ssh.Client
	sftpClient   *sftp.Client
	host         string
	user         string
	instanceName string
	timeout      time.Duration
}

// escapeNewlines escapes newline characters for proper log formatting
func escapeNewlines(s string) string {
	return strings.ReplaceAll(s, "\n", "\\n")
}

// safeClose safely closes a resource and logs any errors
func safeClose(name string, closer func() error) {
	if err := closer(); err != nil {
		logging.Logger().Warn("failed to close resource",
			zap.String("resource", name),
			zap.Error(err))
	}
}

// SSHConfig holds configuration for SSH connection
type SSHConfig struct {
	Host           string
	User           string
	PrivateKey     string // PEM-encoded private key content (preferred)
	PrivateKeyPath string // Path to private key file (deprecated)
	Timeout        time.Duration
	SSHTimeout     time.Duration
	InstanceName   string
}

// NewSSH creates a new SSH connection
func NewSSH(config SSHConfig) (*SSH, error) {
	// Wait for SSH port to become available
	err := waitForSSH(config.Host, config.Timeout)
	if err != nil {
		return nil, fmt.Errorf("SSH not available after timeout: %w", err)
	}

	// Load private key - prefer content over path
	var signer ssh.Signer
	if config.PrivateKey != "" {
		signer, err = parsePrivateKey(config.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
	} else if config.PrivateKeyPath != "" {
		signer, err = loadPrivateKeyFromFile(config.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load private key from file: %w", err)
		}
	} else {
		return nil, fmt.Errorf("either PrivateKey or PrivateKeyPath must be provided")
	}

	// Create SSH client
	clientConfig := &ssh.ClientConfig{
		User: config.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // For demo purposes
		Timeout:         config.SSHTimeout,
	}

	client, err := ssh.Dial("tcp", net.JoinHostPort(config.Host, "22"), clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial SSH: %w", err)
	}

	logging.Logger().Info("SSH connection established",
		zap.String("user", config.User),
		zap.String("host", config.Host),
		zap.String("instance_name", config.InstanceName))

	// Create SFTP client
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create SFTP client: %w", err)
	}

	return &SSH{
		client:       client,
		sftpClient:   sftpClient,
		host:         config.Host,
		user:         config.User,
		instanceName: config.InstanceName,
		timeout:      config.Timeout,
	}, nil
}

// Close closes the SFTP and SSH connections
func (s *SSH) Close() error {
	if s.sftpClient != nil {
		safeClose("SFTP client", s.sftpClient.Close)
	}
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

// GetInstanceName returns the instance name
func (s *SSH) GetInstanceName() string {
	return s.instanceName
}

// Run executes a command on the remote host
func (s *SSH) Run(command string) error {
	session, err := s.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer safeClose("SSH session", session.Close)

	// Create buffers to capture output
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	logging.Logger().Debug("Executing command",
		zap.String("command", logging.Truncate(command)),
		zap.String("host", s.host),
		zap.String("instance_name", s.instanceName))

	// Execute command and capture output
	err = session.Run(command)

	// Get output as strings
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	// Log structured output with truncation and escaped newlines
	logging.Logger().Info("Command executed",
		zap.String("command", logging.Truncate(command)),
		zap.String("host", s.host),
		zap.String("instance_name", s.instanceName),
		zap.String("stdout", escapeNewlines(logging.Truncate(stdoutStr))),
		zap.String("stderr", escapeNewlines(logging.Truncate(stderrStr))),
		zap.Bool("success", err == nil))

	return err
}

// OpenFile opens a remote file for reading and/or writing.
// Uses standard os flags: os.O_RDONLY, os.O_WRONLY, os.O_RDWR, os.O_CREATE, os.O_TRUNC, etc.
func (s *SSH) OpenFile(path string, flags int) (io.ReadWriteCloser, error) {
	file, err := s.sftpClient.OpenFile(path, flags)
	if err != nil {
		return nil, fmt.Errorf("failed to open remote file %s: %w", path, err)
	}

	logging.Logger().Debug("Opened remote file",
		zap.String("path", path),
		zap.Int("flags", flags),
		zap.String("host", s.host))

	return file, nil
}

// Sync copies a file or directory from remote host to local machine using SFTP.
// Automatically detects whether the path is a file or directory and handles accordingly.
func (s *SSH) Sync(remotePath, localPath string) error {
	logging.Logger().Debug("Syncing path using SFTP",
		zap.String("remote_path", remotePath),
		zap.String("local_path", localPath),
		zap.String("host", s.host),
		zap.String("instance_name", s.instanceName))

	// Get remote file info to determine if it's a directory or file
	remoteInfo, err := s.sftpClient.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("failed to stat remote path: %w", err)
	}

	if remoteInfo.IsDir() {
		return s.syncDirectory(remotePath, localPath)
	}
	return s.syncFile(remotePath, localPath, remoteInfo)
}

// copyFile copies a single file from remote to local
func (s *SSH) copyFile(remotePath, localPath string, fileMode os.FileMode) (int64, error) {
	// Create parent directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return 0, fmt.Errorf("failed to create local directory: %w", err)
	}

	// Open remote file
	remoteFile, err := s.sftpClient.Open(remotePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open remote file: %w", err)
	}
	defer safeClose("remote file", remoteFile.Close)

	// Create local file
	localFile, err := os.Create(localPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create local file: %w", err)
	}
	defer safeClose("local file", localFile.Close)

	// Copy file content
	bytesWritten, err := localFile.ReadFrom(remoteFile)
	if err != nil {
		return 0, fmt.Errorf("failed to copy file content: %w", err)
	}

	// Set file permissions
	if err := os.Chmod(localPath, fileMode); err != nil {
		logging.Logger().Warn("failed to set file permissions",
			zap.String("path", localPath),
			zap.Error(err))
	}

	return bytesWritten, nil
}

// syncFile copies a single file from remote to local
func (s *SSH) syncFile(remotePath, localPath string, remoteInfo os.FileInfo) error {
	bytesWritten, err := s.copyFile(remotePath, localPath, remoteInfo.Mode())
	if err != nil {
		return err
	}

	logging.Logger().Info("File synced successfully using SFTP",
		zap.String("remote_path", remotePath),
		zap.String("local_path", localPath),
		zap.String("host", s.host),
		zap.String("instance_name", s.instanceName),
		zap.Int64("size_bytes", bytesWritten))

	return nil
}

// syncDirectory recursively copies a directory from remote to local
func (s *SSH) syncDirectory(remotePath, localPath string) error {
	// Create root local directory
	if err := os.MkdirAll(localPath, 0755); err != nil {
		return fmt.Errorf("failed to create local directory: %w", err)
	}

	// Track statistics
	var filesCopied, dirsCreated int64
	var totalBytes int64

	// Walk through remote directory recursively
	walker := s.sftpClient.Walk(remotePath)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return fmt.Errorf("failed to walk remote directory: %w", err)
		}

		remoteFilePath := walker.Path()
		info := walker.Stat()

		// Calculate relative path from the remote root
		relPath, err := filepath.Rel(remotePath, remoteFilePath)
		if err != nil {
			return fmt.Errorf("failed to calculate relative path: %w", err)
		}

		// Skip root directory entry (already created)
		if relPath == "." {
			continue
		}

		// Build local file path
		localFilePath := filepath.Join(localPath, relPath)

		if info.IsDir() {
			// Create local directory with original permissions
			// MkdirAll creates all parent directories if needed
			if err := os.MkdirAll(localFilePath, info.Mode()); err != nil {
				return fmt.Errorf("failed to create local directory: %w", err)
			}
			dirsCreated++
		} else {
			// Copy file using shared helper function
			bytesWritten, err := s.copyFile(remoteFilePath, localFilePath, info.Mode())
			if err != nil {
				return err
			}
			filesCopied++
			totalBytes += bytesWritten
		}
	}

	logging.Logger().Info("Directory synced successfully using SFTP",
		zap.String("remote_path", remotePath),
		zap.String("local_path", localPath),
		zap.String("host", s.host),
		zap.String("instance_name", s.instanceName),
		zap.Int64("files_copied", filesCopied),
		zap.Int64("dirs_created", dirsCreated),
		zap.Int64("total_bytes", totalBytes))

	return nil
}

// waitForSSH waits for SSH port to become available with timeout
func waitForSSH(host string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, "22"), 5*time.Second)
		if err == nil {
			if closeErr := conn.Close(); closeErr != nil {
				logging.Logger().Debug("failed to close connection test",
					zap.String("host", host),
					zap.Error(closeErr))
			}
			return nil
		}

		// Wait 10 seconds before next attempt
		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("SSH port not available after %v timeout", timeout)
}

// parsePrivateKey parses SSH private key from PEM-encoded string
func parsePrivateKey(privateKeyPEM string) (ssh.Signer, error) {
	signer, err := ssh.ParsePrivateKey([]byte(privateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}
	return signer, nil
}

// loadPrivateKeyFromFile loads SSH private key from file
func loadPrivateKeyFromFile(privateKeyPath string) (ssh.Signer, error) {
	keyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}
	return parsePrivateKey(string(keyBytes))
}
