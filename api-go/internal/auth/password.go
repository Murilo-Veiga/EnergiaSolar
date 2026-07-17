// Package auth reúne tudo relacionado a autenticação e segurança de
// segredo: hash de senha (bcrypt), sessão (JWT em cookie httpOnly) e
// cifragem de credencial de inversor em repouso (AES-GCM).
package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword gera o hash bcrypt de uma senha em texto puro, pra guardar
// no lugar da senha em si (users.password_hash).
func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(hash), err
}

// CheckPassword confere se a senha em texto puro corresponde ao hash
// gravado. Retorna false tanto pra senha errada quanto pra hash inválido —
// quem chama não precisa (nem deve) distinguir os dois casos.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
