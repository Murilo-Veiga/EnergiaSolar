package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

// EncryptCredential e DecryptCredential protegem credenciais de terceiro
// (usuário/senha Huawei, API key FoxESS) em repouso no Postgres — nunca
// gravadas em texto puro. Chave simétrica única (CONFIG_ENCRYPTION_KEY, 32
// bytes) compartilhada entre api-go e collector-go.

// EncryptCredential cifra uma credencial em texto puro (AES-256-GCM) antes
// de gravar em inverter_credentials.credentials_encrypted. O nonce vai
// prefixado no próprio retorno — não precisa ser guardado à parte.
func EncryptCredential(plaintext string, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// DecryptCredential reverte EncryptCredential, extraindo o nonce prefixado
// no início do ciphertext.
func DecryptCredential(ciphertext []byte, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("credencial cifrada corrompida ou curta demais")
	}
	nonce, sealed := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
