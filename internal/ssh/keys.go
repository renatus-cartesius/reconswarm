package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// KeyPair represents an SSH key pair
type KeyPair struct {
	PrivateKeyPath string
	PublicKeyPath  string
	PublicKey      string
}

// GetOrGenerateKeyPair gets existing SSH key pair or generates a new one if it doesn't exist
func GetOrGenerateKeyPair(keyDir string) (*KeyPair, error) {
	// Create key directory if it doesn't exist
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create key directory: %v", err)
	}

	privateKeyPath := filepath.Join(keyDir, "reconswarm_key")
	publicKeyPath := filepath.Join(keyDir, "reconswarm_key.pub")

	// Check if private key already exists
	if _, err := os.Stat(privateKeyPath); err == nil {
		// Private key exists, check if public key exists too
		if _, err := os.Stat(publicKeyPath); err == nil {
			// Both keys exist, read the public key
			publicKeyBytes, err := os.ReadFile(publicKeyPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read existing public key: %v", err)
			}

			return &KeyPair{
				PrivateKeyPath: privateKeyPath,
				PublicKeyPath:  publicKeyPath,
				PublicKey:      string(publicKeyBytes),
			}, nil
		}
		// Private key exists but public key doesn't, regenerate public key
		return generatePublicKeyFromPrivate(privateKeyPath, publicKeyPath)
	}

	// Keys don't exist, generate new key pair
	return generateNewKeyPair(privateKeyPath, publicKeyPath)
}

// generateNewKeyPair generates a completely new SSH key pair
func generateNewKeyPair(privateKeyPath, publicKeyPath string) (*KeyPair, error) {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %v", err)
	}

	// Create private key file
	privateKeyFile, err := os.Create(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create private key file: %v", err)
	}
	defer privateKeyFile.Close()

	// Encode private key to PEM format
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
		return nil, fmt.Errorf("failed to encode private key: %v", err)
	}

	// Set proper permissions for private key
	if err := os.Chmod(privateKeyPath, 0600); err != nil {
		return nil, fmt.Errorf("failed to set private key permissions: %v", err)
	}

	// Generate public key
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate public key: %v", err)
	}

	// Create public key file
	publicKeyFile, err := os.Create(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create public key file: %v", err)
	}
	defer publicKeyFile.Close()

	// Write public key in OpenSSH format
	publicKeyString := string(ssh.MarshalAuthorizedKey(publicKey))
	if _, err := publicKeyFile.WriteString(publicKeyString); err != nil {
		return nil, fmt.Errorf("failed to write public key: %v", err)
	}

	// Set proper permissions for public key
	if err := os.Chmod(publicKeyPath, 0644); err != nil {
		return nil, fmt.Errorf("failed to set public key permissions: %v", err)
	}

	return &KeyPair{
		PrivateKeyPath: privateKeyPath,
		PublicKeyPath:  publicKeyPath,
		PublicKey:      publicKeyString,
	}, nil
}

// generatePublicKeyFromPrivate generates public key from existing private key
func generatePublicKeyFromPrivate(privateKeyPath, publicKeyPath string) (*KeyPair, error) {
	// Read private key
	privateKeyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %v", err)
	}

	// Parse private key
	block, _ := pem.Decode(privateKeyBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to decode private key")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	// Generate public key
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate public key: %v", err)
	}

	// Create public key file
	publicKeyFile, err := os.Create(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create public key file: %v", err)
	}
	defer publicKeyFile.Close()

	// Write public key in OpenSSH format
	publicKeyString := string(ssh.MarshalAuthorizedKey(publicKey))
	if _, err := publicKeyFile.WriteString(publicKeyString); err != nil {
		return nil, fmt.Errorf("failed to write public key: %v", err)
	}

	// Set proper permissions for public key
	if err := os.Chmod(publicKeyPath, 0644); err != nil {
		return nil, fmt.Errorf("failed to set public key permissions: %v", err)
	}

	return &KeyPair{
		PrivateKeyPath: privateKeyPath,
		PublicKeyPath:  publicKeyPath,
		PublicKey:      publicKeyString,
	}, nil
}

// Cleanup removes the generated key files
func (kp *KeyPair) Cleanup() error {
	if err := os.Remove(kp.PrivateKeyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove private key: %v", err)
	}
	if err := os.Remove(kp.PublicKeyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove public key: %v", err)
	}
	return nil
}
