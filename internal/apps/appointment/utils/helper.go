package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"

	"golang.org/x/crypto/bcrypt"

	"appointment_system/internal/apps/appointment/constant"
)

func HashPassword(password string) ([]byte, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return hashedPassword, nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func NewOpaqueToken() (string, error) {
	var buf [constant.RefreshTokenBytes]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// NewCode returns length characters drawn uniformly from alphabet with
// crypto/rand.
func NewCode(alphabet string, length int) (string, error) {
	limit := big.NewInt(int64(len(alphabet)))
	code := make([]byte, length)
	for i := range code {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		code[i] = alphabet[index.Int64()]
	}
	return string(code), nil
}

func CheckPassword(passwordHash []byte, password string) error {
	return bcrypt.CompareHashAndPassword(passwordHash, []byte(password))
}
