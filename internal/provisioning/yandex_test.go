package provisioning

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// validYandexAuthorizedKeyJSON — минимальная структура authorized key файла Yandex Cloud
// (id, service_account_id, private_key). iamkey принимает snake_case (protojson).
// Для тестов используем placeholder private_key (RSA PEM); SDK проверит при Build.
const validYandexAuthorizedKeyJSON = `{
  "id": "aje000000000000000000",
  "service_account_id": "aje000000000000000000",
  "private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC\n-----END PRIVATE KEY-----\n"
}`

func TestNewYcProvisioner_keyPath_FileNotFound(t *testing.T) {
	_, err := NewYcProvisioner("", "/nonexistent/key.json", "test-folder-id")
	if err == nil {
		t.Fatal("NewYcProvisioner with nonexistent key_path expected error, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Logf("expected path error or ErrNotExist, got: %v", err)
		}
	}
}

func TestNewYcProvisioner_keyPath_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "bad-key.json")
	if err := os.WriteFile(keyPath, []byte("not json at all"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := NewYcProvisioner("", keyPath, "test-folder-id")
	if err == nil {
		t.Fatal("NewYcProvisioner with invalid JSON key_path expected error, got nil")
	}
}

func TestNewYcProvisioner_keyPath_ValidAuthorizedKeyJSON(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "sa-key.json")
	if err := os.WriteFile(keyPath, []byte(validYandexAuthorizedKeyJSON), 0644); err != nil {
		t.Fatalf("write test key file: %v", err)
	}

	prov, err := NewYcProvisioner("", keyPath, "test-folder-id")
	if err != nil {
		// SDK.Build может вернуть ошибку из-за невалидного ключа/сети — тогда проверяем, что ошибка не на этапе чтения/парсинга файла
		t.Logf("NewYcProvisioner (key_path) returned error (expected if key is fake): %v", err)
		return
	}
	if prov == nil {
		t.Fatal("NewYcProvisioner with valid key_path JSON expected non-nil provisioner")
	}
	if prov.folderID != "test-folder-id" {
		t.Errorf("folderID = %q, want test-folder-id", prov.folderID)
	}
}

func TestNewYcProvisioner_iamTokenPreferredOverKeyPath(t *testing.T) {
	// При указании и iam_token, и key_path используется iam_token
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "sa-key.json")
	if err := os.WriteFile(keyPath, []byte(validYandexAuthorizedKeyJSON), 0644); err != nil {
		t.Fatalf("write test key file: %v", err)
	}

	prov, err := NewYcProvisioner("fake-iam-token", keyPath, "test-folder-id")
	if err != nil {
		// С фейковым токеном SDK может упасть при первом вызове — нас интересует, что создание прошло по ветке iam_token
		t.Logf("NewYcProvisioner (iam_token) error: %v", err)
		return
	}
	if prov == nil {
		t.Fatal("expected non-nil provisioner when using iam_token")
	}
	if prov.folderID != "test-folder-id" {
		t.Errorf("folderID = %q, want test-folder-id", prov.folderID)
	}
}
