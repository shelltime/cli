package model

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestNewAESGCMService(t *testing.T) {
	service := NewAESGCMService()
	if service == nil {
		t.Fatal("NewAESGCMService returned nil")
	}

	aesService, ok := service.(*AESGCMService)
	if !ok {
		t.Fatal("NewAESGCMService did not return an *AESGCMService")
	}

	if aesService.KeySize != 32 {
		t.Errorf("Expected KeySize to be 32, got %d", aesService.KeySize)
	}
}

func TestAESGCMService_GenerateKeys(t *testing.T) {
	service := NewAESGCMService()

	publicKey, privateKey, err := service.GenerateKeys()
	if err != nil {
		t.Fatalf("GenerateKeys failed: %v", err)
	}

	// AES is symmetric, so private key should be nil
	if privateKey != nil {
		t.Error("Expected private key to be nil for AES")
	}

	// Public key should be base64 encoded
	if publicKey == nil {
		t.Fatal("Expected public key to be non-nil")
	}

	// Verify it's valid base64
	decoded, err := base64.StdEncoding.DecodeString(string(publicKey))
	if err != nil {
		t.Fatalf("Public key is not valid base64: %v", err)
	}

	// Decoded key should be 32 bytes (256 bits)
	if len(decoded) != 32 {
		t.Errorf("Expected decoded key to be 32 bytes, got %d", len(decoded))
	}
}

func TestAESGCMService_GenerateKeys_Uniqueness(t *testing.T) {
	service := NewAESGCMService()

	key1, _, err := service.GenerateKeys()
	if err != nil {
		t.Fatalf("First GenerateKeys failed: %v", err)
	}

	key2, _, err := service.GenerateKeys()
	if err != nil {
		t.Fatalf("Second GenerateKeys failed: %v", err)
	}

	// Keys should be unique
	if string(key1) == string(key2) {
		t.Error("Generated keys should be unique")
	}
}

func TestAESGCMService_Encrypt(t *testing.T) {
	service := NewAESGCMService()

	// Generate a key
	key, _, err := service.GenerateKeys()
	if err != nil {
		t.Fatalf("GenerateKeys failed: %v", err)
	}

	plaintext := []byte("Hello, World! This is a test message for AES-GCM encryption.")

	ciphertext, nonce, err := service.Encrypt(string(key), plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Ciphertext should not be nil or empty
	if ciphertext == nil || len(ciphertext) == 0 {
		t.Error("Ciphertext should not be empty")
	}

	// Nonce should be 12 bytes (GCM standard)
	if len(nonce) != 12 {
		t.Errorf("Expected nonce to be 12 bytes, got %d", len(nonce))
	}

	// Ciphertext should be different from plaintext
	if string(ciphertext) == string(plaintext) {
		t.Error("Ciphertext should be different from plaintext")
	}
}

func TestAESGCMService_Encrypt_UniqueNonce(t *testing.T) {
	service := NewAESGCMService()

	key, _, err := service.GenerateKeys()
	if err != nil {
		t.Fatalf("GenerateKeys failed: %v", err)
	}

	plaintext := []byte("Test message")

	_, nonce1, err := service.Encrypt(string(key), plaintext)
	if err != nil {
		t.Fatalf("First Encrypt failed: %v", err)
	}

	_, nonce2, err := service.Encrypt(string(key), plaintext)
	if err != nil {
		t.Fatalf("Second Encrypt failed: %v", err)
	}

	// Nonces should be unique
	if string(nonce1) == string(nonce2) {
		t.Error("Nonces should be unique for each encryption")
	}
}

func TestAESGCMService_Encrypt_InvalidKey(t *testing.T) {
	service := NewAESGCMService()

	testCases := []struct {
		name string
		key  string
	}{
		{"invalid base64", "not-valid-base64!@#$"},
		{"empty key", ""},
		{"wrong size key", base64.StdEncoding.EncodeToString([]byte("short"))},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := service.Encrypt(tc.key, []byte("test"))
			if err == nil {
				t.Error("Expected error for invalid key")
			}
		})
	}
}

func TestAESGCMService_Encrypt_EmptyPlaintext(t *testing.T) {
	service := NewAESGCMService()

	key, _, err := service.GenerateKeys()
	if err != nil {
		t.Fatalf("GenerateKeys failed: %v", err)
	}

	// Empty plaintext should still work (GCM can encrypt empty messages)
	ciphertext, nonce, err := service.Encrypt(string(key), []byte{})
	if err != nil {
		t.Fatalf("Encrypt with empty plaintext failed: %v", err)
	}

	if nonce == nil {
		t.Error("Nonce should not be nil even for empty plaintext")
	}

	// Ciphertext will include the auth tag, so it won't be empty
	if ciphertext == nil {
		t.Error("Ciphertext should not be nil")
	}
}

func TestAESGCMService_Encrypt_LargePlaintext(t *testing.T) {
	service := NewAESGCMService()

	key, _, err := service.GenerateKeys()
	if err != nil {
		t.Fatalf("GenerateKeys failed: %v", err)
	}

	// Create a large plaintext (1MB)
	largePlaintext := make([]byte, 1024*1024)
	for i := range largePlaintext {
		largePlaintext[i] = byte(i % 256)
	}

	ciphertext, nonce, err := service.Encrypt(string(key), largePlaintext)
	if err != nil {
		t.Fatalf("Encrypt with large plaintext failed: %v", err)
	}

	if len(ciphertext) < len(largePlaintext) {
		t.Error("Ciphertext should be at least as long as plaintext (plus auth tag)")
	}

	if len(nonce) != 12 {
		t.Errorf("Expected nonce to be 12 bytes, got %d", len(nonce))
	}
}

func TestAESGCMService_Decrypt_Panics(t *testing.T) {
	service := NewAESGCMService()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected Decrypt to panic")
		}
	}()

	// This should panic as Decrypt is not implemented
	service.Decrypt("key", []byte("ciphertext"), []byte("nonce"))
}

// RSA Service Tests

func TestNewRSAService(t *testing.T) {
	service := NewRSAService()
	if service == nil {
		t.Fatal("NewRSAService returned nil")
	}

	rsaService, ok := service.(*RSAService)
	if !ok {
		t.Fatal("NewRSAService did not return an *RSAService")
	}

	if rsaService.KeySize != 2048 {
		t.Errorf("Expected KeySize to be 2048, got %d", rsaService.KeySize)
	}
}

func TestRSAService_GenerateKeys(t *testing.T) {
	service := NewRSAService()

	publicKey, privateKey, err := service.GenerateKeys()
	if err != nil {
		t.Fatalf("GenerateKeys failed: %v", err)
	}

	// Both keys should be non-nil
	if publicKey == nil {
		t.Error("Expected public key to be non-nil")
	}
	if privateKey == nil {
		t.Error("Expected private key to be non-nil")
	}

	// Verify PEM format for public key
	pubKeyStr := string(publicKey)
	if !strings.Contains(pubKeyStr, "-----BEGIN RSA PUBLIC KEY-----") {
		t.Error("Public key should be in PEM format")
	}
	if !strings.Contains(pubKeyStr, "-----END RSA PUBLIC KEY-----") {
		t.Error("Public key should have proper PEM ending")
	}

	// Verify PEM format for private key
	privKeyStr := string(privateKey)
	if !strings.Contains(privKeyStr, "-----BEGIN RSA PRIVATE KEY-----") {
		t.Error("Private key should be in PEM format")
	}
	if !strings.Contains(privKeyStr, "-----END RSA PRIVATE KEY-----") {
		t.Error("Private key should have proper PEM ending")
	}
}

func TestRSAService_GenerateKeys_Uniqueness(t *testing.T) {
	service := NewRSAService()

	pub1, priv1, err := service.GenerateKeys()
	if err != nil {
		t.Fatalf("First GenerateKeys failed: %v", err)
	}

	pub2, priv2, err := service.GenerateKeys()
	if err != nil {
		t.Fatalf("Second GenerateKeys failed: %v", err)
	}

	// Keys should be unique
	if string(pub1) == string(pub2) {
		t.Error("Generated public keys should be unique")
	}
	if string(priv1) == string(priv2) {
		t.Error("Generated private keys should be unique")
	}
}

func TestRSAService_Encrypt(t *testing.T) {
	service := NewRSAService()

	publicKey, _, err := service.GenerateKeys()
	if err != nil {
		t.Fatalf("GenerateKeys failed: %v", err)
	}

	plaintext := []byte("Hello, RSA!")

	ciphertext, nonce, err := service.Encrypt(string(publicKey), plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// For RSA, nonce should be nil
	if nonce != nil {
		t.Error("Expected nonce to be nil for RSA encryption")
	}

	// Ciphertext should not be nil or empty
	if ciphertext == nil || len(ciphertext) == 0 {
		t.Error("Ciphertext should not be empty")
	}

	// Ciphertext should be different from plaintext
	if string(ciphertext) == string(plaintext) {
		t.Error("Ciphertext should be different from plaintext")
	}
}

func TestRSAService_Encrypt_UniqueOutput(t *testing.T) {
	service := NewRSAService()

	publicKey, _, err := service.GenerateKeys()
	if err != nil {
		t.Fatalf("GenerateKeys failed: %v", err)
	}

	plaintext := []byte("Test message")

	cipher1, _, err := service.Encrypt(string(publicKey), plaintext)
	if err != nil {
		t.Fatalf("First Encrypt failed: %v", err)
	}

	cipher2, _, err := service.Encrypt(string(publicKey), plaintext)
	if err != nil {
		t.Fatalf("Second Encrypt failed: %v", err)
	}

	// Due to PKCS1v15 padding randomness, ciphertexts should be unique
	if string(cipher1) == string(cipher2) {
		t.Error("RSA ciphertexts should be unique due to padding")
	}
}

func TestRSAService_Encrypt_InvalidKey(t *testing.T) {
	service := NewRSAService()

	testCases := []struct {
		name string
		key  string
	}{
		{"empty key", ""},
		{"invalid PEM", "not a valid PEM key"},
		{"malformed PEM", "-----BEGIN RSA PUBLIC KEY-----\ninvalid content\n-----END RSA PUBLIC KEY-----"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := service.Encrypt(tc.key, []byte("test"))
			if err == nil {
				t.Error("Expected error for invalid key")
			}
		})
	}
}

func TestRSAService_Encrypt_MessageTooLong(t *testing.T) {
	service := NewRSAService()

	publicKey, _, err := service.GenerateKeys()
	if err != nil {
		t.Fatalf("GenerateKeys failed: %v", err)
	}

	// RSA PKCS1v15 with 2048-bit key can encrypt at most 245 bytes
	// Create a message that's too long
	longMessage := make([]byte, 300)
	for i := range longMessage {
		longMessage[i] = 'a'
	}

	_, _, err = service.Encrypt(string(publicKey), longMessage)
	if err == nil {
		t.Error("Expected error for message too long")
	}
}

func TestRSAService_Decrypt_Panics(t *testing.T) {
	service := NewRSAService()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected Decrypt to panic")
		}
	}()

	// This should panic as Decrypt is not implemented
	service.Decrypt("key", []byte("ciphertext"), nil)
}

// CryptoService interface compliance tests

func TestAESGCMService_ImplementsCryptoService(t *testing.T) {
	var _ CryptoService = &AESGCMService{}
}

func TestRSAService_ImplementsCryptoService(t *testing.T) {
	var _ CryptoService = &RSAService{}
}
