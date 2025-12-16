package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// KeyPair represents an SSH key pair
type KeyPair struct {
	PrivateKey string // PEM-encoded private key
	PublicKey  string // OpenSSH format public key
}

// GenerateKeyPairInMemory generates a new SSH key pair and returns it in memory
// without writing to disk. Keys are stored in etcd via KeyProvider.
func GenerateKeyPairInMemory() (*KeyPair, error) {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Encode private key to PEM format
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	privateKeyString := string(pem.EncodeToMemory(privateKeyPEM))

	// Generate public key
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate public key: %w", err)
	}
	publicKeyString := string(ssh.MarshalAuthorizedKey(publicKey))

	return &KeyPair{
		PrivateKey: privateKeyString,
		PublicKey:  publicKeyString,
	}, nil
}

// FromStrings creates a KeyPair from private and public key strings
func FromStrings(privateKey, publicKey string) *KeyPair {
	return &KeyPair{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	}
}
