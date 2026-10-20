package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("неверный логин или пароль")

// HashPassword создаёт bcrypt-хеш (для cmd/hashpw и тестов).
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword сравнивает пароль с bcrypt-хешем за постоянное время библиотеки.
func CheckPassword(hash, password string) error {
	if hash == "" || password == "" {
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}
