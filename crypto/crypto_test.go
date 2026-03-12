package crypto

import (
	"crypto/rand"
	"testing"
)

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if len(token) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("expected 64 hex chars, got %d", len(token))
	}

	// Two tokens should differ
	token2, _ := GenerateToken()
	if token == token2 {
		t.Error("two tokens should not be identical")
	}
}

func TestHashToken(t *testing.T) {
	hash := HashToken("test-token")
	if len(hash) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(hash))
	}

	// Same input = same hash
	if HashToken("test-token") != hash {
		t.Error("same input should produce same hash")
	}

	// Different input = different hash
	if HashToken("other-token") == hash {
		t.Error("different input should produce different hash")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	masterKey := make([]byte, 32)
	rand.Read(masterKey)

	plaintext := "sk-ant-api03-secret-key-value"
	ciphertext, err := Encrypt(plaintext, masterKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decrypted, err := Decrypt(ciphertext, masterKey)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	rand.Read(key1)
	rand.Read(key2)

	ciphertext, _ := Encrypt("secret", key1)
	_, err := Decrypt(ciphertext, key2)
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestDecryptTooShort(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	_, err := Decrypt([]byte("short"), key)
	if err == nil {
		t.Error("expected error for short ciphertext")
	}
}
