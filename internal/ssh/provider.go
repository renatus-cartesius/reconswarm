package ssh

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"reconswarm/internal/logging"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

const sshKeysPath = "/config/ssh_keys"

// KeyProvider defines the interface for SSH key management
type KeyProvider interface {
	// GetOrCreate retrieves existing keys or creates new ones
	GetOrCreate(ctx context.Context) (*KeyPair, error)
	// Save saves the key pair to storage
	Save(ctx context.Context, keyPair *KeyPair) error
	// Delete removes the keys from storage
	Delete(ctx context.Context) error
	// Close closes any connections
	Close() error
}

// EtcdKeyProvider stores SSH keys in etcd
type EtcdKeyProvider struct {
	client *clientv3.Client
}

// NewEtcdKeyProvider creates a new etcd-based key provider
func NewEtcdKeyProvider(endpoints []string) (*EtcdKeyProvider, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}
	return &EtcdKeyProvider{client: cli}, nil
}

// GetOrCreate retrieves existing keys from etcd or creates new ones
func (p *EtcdKeyProvider) GetOrCreate(ctx context.Context) (*KeyPair, error) {
	// Try to get existing keys
	resp, err := p.client.Get(ctx, sshKeysPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH keys from etcd: %w", err)
	}

	if len(resp.Kvs) > 0 {
		// Keys found, unmarshal and return
		var stored storedKeyPair
		if err := json.Unmarshal(resp.Kvs[0].Value, &stored); err != nil {
			return nil, fmt.Errorf("failed to unmarshal SSH keys: %w", err)
		}
		logging.Logger().Info("Using existing SSH keys from etcd")
		return &KeyPair{
			PrivateKey: stored.PrivateKey,
			PublicKey:  stored.PublicKey,
		}, nil
	}

	// No keys found, generate new ones
	logging.Logger().Info("No SSH keys found in etcd, generating new key pair")
	keyPair, err := GenerateKeyPairInMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to generate SSH key pair: %w", err)
	}

	// Save to etcd
	if err := p.Save(ctx, keyPair); err != nil {
		return nil, fmt.Errorf("failed to save SSH keys to etcd: %w", err)
	}

	logging.Logger().Info("SSH keys generated and stored in etcd")
	return keyPair, nil
}

// Save saves the key pair to etcd
func (p *EtcdKeyProvider) Save(ctx context.Context, keyPair *KeyPair) error {
	stored := storedKeyPair{
		PrivateKey: keyPair.PrivateKey,
		PublicKey:  keyPair.PublicKey,
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("failed to marshal SSH keys: %w", err)
	}
	_, err = p.client.Put(ctx, sshKeysPath, string(data))
	if err != nil {
		return fmt.Errorf("failed to save SSH keys to etcd: %w", err)
	}
	return nil
}

// Delete removes the keys from etcd
func (p *EtcdKeyProvider) Delete(ctx context.Context) error {
	_, err := p.client.Delete(ctx, sshKeysPath)
	if err != nil {
		return fmt.Errorf("failed to delete SSH keys from etcd: %w", err)
	}
	return nil
}

// Close closes the etcd client
func (p *EtcdKeyProvider) Close() error {
	return p.client.Close()
}

// storedKeyPair represents the JSON structure stored in etcd
type storedKeyPair struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

// InMemoryKeyProvider generates keys in memory (no persistence)
type InMemoryKeyProvider struct {
	keyPair *KeyPair
}

// NewInMemoryKeyProvider creates a new in-memory key provider
func NewInMemoryKeyProvider() *InMemoryKeyProvider {
	return &InMemoryKeyProvider{}
}

// GetOrCreate generates a new key pair in memory
func (p *InMemoryKeyProvider) GetOrCreate(ctx context.Context) (*KeyPair, error) {
	if p.keyPair != nil {
		return p.keyPair, nil
	}

	logging.Logger().Info("Generating SSH key pair in memory")
	keyPair, err := GenerateKeyPairInMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to generate SSH key pair: %w", err)
	}
	p.keyPair = keyPair
	return keyPair, nil
}

// Save is a no-op for in-memory provider
func (p *InMemoryKeyProvider) Save(ctx context.Context, keyPair *KeyPair) error {
	p.keyPair = keyPair
	return nil
}

// Delete clears the in-memory key pair
func (p *InMemoryKeyProvider) Delete(ctx context.Context) error {
	p.keyPair = nil
	return nil
}

// Close is a no-op for in-memory provider
func (p *InMemoryKeyProvider) Close() error {
	return nil
}

// NewKeyProvider creates the appropriate key provider based on etcd availability
func NewKeyProvider(etcdEndpoints []string) KeyProvider {
	if len(etcdEndpoints) == 0 {
		logging.Logger().Info("No etcd endpoints configured, using in-memory key provider")
		return NewInMemoryKeyProvider()
	}

	provider, err := NewEtcdKeyProvider(etcdEndpoints)
	if err != nil {
		logging.Logger().Warn("Failed to connect to etcd, falling back to in-memory key provider",
			zap.Error(err))
		return NewInMemoryKeyProvider()
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = provider.client.Get(ctx, "/test_connection")
	if err != nil {
		logging.Logger().Warn("etcd connection test failed, falling back to in-memory key provider",
			zap.Error(err))
		provider.Close()
		return NewInMemoryKeyProvider()
	}

	logging.Logger().Info("Connected to etcd for SSH key storage",
		zap.Strings("endpoints", etcdEndpoints))
	return provider
}

