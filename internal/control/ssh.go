package control

import (
	"bytes"
	"fmt"
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
	client         *ssh.Client
	host           string
	user           string
	instanceName   string
	privateKeyPath string
	timeout        time.Duration
}

// escapeNewlines escapes newline characters for proper log formatting
func escapeNewlines(s string) string {
	return strings.ReplaceAll(s, "\n", "\\n")
}

// SSHConfig holds configuration for SSH connection
type SSHConfig struct {
	Host         string
	User         string
	PrivateKey   string
	Timeout      time.Duration
	SSHTimeout   time.Duration
	InstanceName string
}

// NewSSH creates a new SSH connection
func NewSSH(config SSHConfig) (*SSH, error) {
	// Wait for SSH port to become available
	err := waitForSSH(config.Host, config.Timeout)
	if err != nil {
		return nil, fmt.Errorf("SSH not available after timeout: %v", err)
	}

	// Load private key
	signer, err := loadPrivateKey(config.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %v", err)
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
		return nil, fmt.Errorf("failed to dial SSH: %v", err)
	}

	logging.Logger().Info("SSH connection established",
		zap.String("user", config.User),
		zap.String("host", config.Host),
		zap.String("instance_name", config.InstanceName))

	return &SSH{
		client:         client,
		host:           config.Host,
		user:           config.User,
		instanceName:   config.InstanceName,
		privateKeyPath: config.PrivateKey,
		timeout:        config.Timeout,
	}, nil
}

// Close closes the SSH connection
func (s *SSH) Close() error {
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
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Create buffers to capture output
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	logging.Logger().Debug("Executing command",
		zap.String("command", command),
		zap.String("host", s.host),
		zap.String("instance_name", s.instanceName))

	// Execute command and capture output
	err = session.Run(command)

	// Get output as strings
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	// Log structured output with escaped newlines
	logging.Logger().Info("Command executed",
		zap.String("command", command),
		zap.String("host", s.host),
		zap.String("instance_name", s.instanceName),
		zap.String("stdout", escapeNewlines(stdoutStr)),
		zap.String("stderr", escapeNewlines(stderrStr)),
		zap.Bool("success", err == nil))

	return err
}

// WriteFile writes content to a file on the remote host
func (s *SSH) WriteFile(remotePath, content string, mode os.FileMode) error {
	// Create a temporary file locally
	tempFile, err := os.CreateTemp("", "ssh_write_*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	// Write content to temp file
	if err := os.WriteFile(tempFile.Name(), []byte(content), mode); err != nil {
		return fmt.Errorf("failed to write to temp file: %v", err)
	}

	// Write file using echo and redirection
	command := fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", remotePath, string(content))
	return s.Run(command)
}

// ReadFile reads a file from the remote host
func (s *SSH) ReadFile(remotePath string) (string, error) {
	session, err := s.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	logging.Logger().Debug("Reading file",
		zap.String("path", remotePath),
		zap.String("host", s.host),
		zap.String("instance_name", s.instanceName))

	output, err := session.CombinedOutput(fmt.Sprintf("cat %s", remotePath))
	outputStr := string(output)

	// Log structured output with escaped newlines
	logging.Logger().Info("File read",
		zap.String("path", remotePath),
		zap.String("host", s.host),
		zap.String("instance_name", s.instanceName),
		zap.Bool("success", err == nil))

	return outputStr, err
}

// SyncFile copies a file from remote host to local machine using SFTP
func (s *SSH) SyncFile(remotePath, localPath string) error {
	logging.Logger().Debug("Syncing file using SFTP",
		zap.String("remote_path", remotePath),
		zap.String("local_path", localPath),
		zap.String("host", s.host),
		zap.String("instance_name", s.instanceName))

	// Create SFTP client
	sftpClient, err := sftp.NewClient(s.client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %v", err)
	}
	defer sftpClient.Close()

	// Create local directory if it doesn't exist
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("failed to create local directory: %v", err)
	}

	// Open remote file
	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file: %v", err)
	}
	defer remoteFile.Close()

	// Create local file
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %v", err)
	}
	defer localFile.Close()

	// Copy file content
	bytesWritten, err := localFile.ReadFrom(remoteFile)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %v", err)
	}

	logging.Logger().Info("File synced successfully using SFTP",
		zap.String("remote_path", remotePath),
		zap.String("local_path", localPath),
		zap.String("host", s.host),
		zap.String("instance_name", s.instanceName),
		zap.Int64("size_bytes", bytesWritten))

	return nil
}

// waitForSSH waits for SSH port to become available with timeout
func waitForSSH(host string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, "22"), 5*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}

		// Wait 10 seconds before next attempt
		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("SSH port not available after %v timeout", timeout)
}

// loadPrivateKey loads SSH private key from file
func loadPrivateKey(privateKeyPath string) (ssh.Signer, error) {
	keyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %v", err)
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	return signer, nil
}
