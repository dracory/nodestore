package nodestore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// CredentialCipher seals the credential value map before it is
// persisted into the entity attribute (encryption at rest) and opens
// it on read. The Node-RED analogue is flows_cred.json's
// credentialSecret.
type CredentialCipher interface {
	// Encrypt returns the sealed (self-describing) form of plaintext.
	Encrypt(plaintext []byte) (string, error)
	// Decrypt opens a sealed value. Implementations must pass
	// through values that are not in their sealed format so legacy
	// plaintext rows still decode.
	Decrypt(sealed string) ([]byte, error)
}

// credentialCipherPrefixV1 marks values sealed by the AES-256-GCM
// implementation below — the version tag lets a future cipher change
// coexist with rows written today.
const credentialCipherPrefixV1 = "v1:"

// aesGCMCredentialCipher seals credential payloads with AES-256-GCM:
// a fresh random nonce per write, prepended to the ciphertext and the
// whole blob base64'd behind the "v1:" prefix.
type aesGCMCredentialCipher struct {
	key []byte
}

// NewAESCredentialCipher returns a CredentialCipher using AES-256-GCM
// with the given 32-byte key. The key is copied — callers may zero
// their buffer afterwards.
func NewAESCredentialCipher(key []byte) (CredentialCipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("credential cipher: key must be 32 bytes (AES-256), got %d", len(key))
	}
	k := make([]byte, 32)
	copy(k, key)
	return &aesGCMCredentialCipher{key: k}, nil
}

func (c *aesGCMCredentialCipher) Encrypt(plaintext []byte) (string, error) {
	gcm, err := c.gcm()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return credentialCipherPrefixV1 + base64.StdEncoding.EncodeToString(sealed), nil
}

func (c *aesGCMCredentialCipher) Decrypt(sealed string) ([]byte, error) {
	if !strings.HasPrefix(sealed, credentialCipherPrefixV1) {
		// Legacy plaintext row — returned as-is so it still decodes;
		// the next CredentialUpdate seals it.
		return []byte(sealed), nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, credentialCipherPrefixV1))
	if err != nil {
		return nil, fmt.Errorf("credential cipher: invalid sealed value: %w", err)
	}
	gcm, err := c.gcm()
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("credential cipher: sealed value too short")
	}
	plaintext, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return nil, fmt.Errorf("credential cipher: decryption failed (wrong key or corrupted data)")
	}
	return plaintext, nil
}

func (c *aesGCMCredentialCipher) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
