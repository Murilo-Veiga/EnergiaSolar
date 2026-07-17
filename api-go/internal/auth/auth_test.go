package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("senha-correta-123")
	if err != nil {
		t.Fatalf("HashPassword retornou erro: %v", err)
	}
	if hash == "senha-correta-123" {
		t.Fatal("HashPassword não deveria devolver a senha em texto puro")
	}
	if !CheckPassword(hash, "senha-correta-123") {
		t.Error("CheckPassword deveria aceitar a senha correta")
	}
	if CheckPassword(hash, "senha-errada") {
		t.Error("CheckPassword não deveria aceitar uma senha errada")
	}
}

func TestCheckPasswordWithEmptyHash(t *testing.T) {
	// Caso real: handleLogin chama CheckPassword mesmo quando o SELECT não
	// encontrou o usuário (passwordHash fica ""), pra não vazar por timing
	// se o e-mail existe. Nunca deve dar panic nem aceitar como válido.
	if CheckPassword("", "qualquer-senha") {
		t.Error("CheckPassword com hash vazio nunca deveria retornar true")
	}
}

func TestIssueAndParseToken(t *testing.T) {
	secret := []byte("segredo-de-teste-32-bytes-aqui!")
	token, err := IssueToken("user-123", secret)
	if err != nil {
		t.Fatalf("IssueToken retornou erro: %v", err)
	}

	claims, err := ParseToken(token, secret)
	if err != nil {
		t.Fatalf("ParseToken retornou erro pra um token recém-emitido: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Errorf("UserID = %q, esperado %q", claims.UserID, "user-123")
	}
}

func TestParseTokenWrongSecret(t *testing.T) {
	token, err := IssueToken("user-123", []byte("segredo-correto-32-bytes-aqui!!"))
	if err != nil {
		t.Fatalf("IssueToken retornou erro: %v", err)
	}
	if _, err := ParseToken(token, []byte("segredo-errado-32-bytes-aqui!!!")); err == nil {
		t.Error("ParseToken deveria rejeitar um token assinado com outro segredo")
	}
}

func TestParseTokenExpired(t *testing.T) {
	secret := []byte("segredo-de-teste-32-bytes-aqui!")
	claims := Claims{
		UserID: "user-123",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		t.Fatalf("falha ao montar token de teste: %v", err)
	}
	if _, err := ParseToken(token, secret); err == nil {
		t.Error("ParseToken deveria rejeitar um token expirado")
	}
}

func TestEncryptDecryptCredentialRoundTrip(t *testing.T) {
	key := []byte("12345678901234567890123456789012") // 32 bytes
	plaintext := `{"username":"marcos","system_code":"solturi123"}`

	ciphertext, err := EncryptCredential(plaintext, key)
	if err != nil {
		t.Fatalf("EncryptCredential retornou erro: %v", err)
	}
	if strings.Contains(string(ciphertext), "marcos") {
		t.Fatal("ciphertext não deveria conter o texto puro em nenhum trecho")
	}

	decrypted, err := DecryptCredential(ciphertext, key)
	if err != nil {
		t.Fatalf("DecryptCredential retornou erro: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("DecryptCredential = %q, esperado %q", decrypted, plaintext)
	}
}

func TestDecryptCredentialWrongKey(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	wrongKey := []byte("00000000000000000000000000000000")[:32]

	ciphertext, err := EncryptCredential("segredo", key)
	if err != nil {
		t.Fatalf("EncryptCredential retornou erro: %v", err)
	}
	if _, err := DecryptCredential(ciphertext, wrongKey); err == nil {
		t.Error("DecryptCredential deveria falhar com a chave errada")
	}
}

func TestDecryptCredentialCorrupted(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	if _, err := DecryptCredential([]byte("curto-demais"), key); err == nil {
		t.Error("DecryptCredential deveria rejeitar ciphertext menor que o nonce")
	}
}
