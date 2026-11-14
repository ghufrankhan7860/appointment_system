package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
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

// NewURLToken returns size random bytes from crypto/rand as unpadded base64url,
// short and safe in a URL path.
func NewURLToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
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

// NewHexID returns size random bytes from crypto/rand, hex encoded.
func NewHexID(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// dammTable is the Damm algorithm's quasigroup of order 10.
var dammTable = [10][10]byte{
	{0, 3, 1, 7, 5, 9, 8, 6, 4, 2},
	{7, 0, 9, 2, 1, 5, 4, 8, 6, 3},
	{4, 2, 0, 6, 8, 7, 1, 3, 5, 9},
	{1, 7, 5, 0, 9, 8, 3, 4, 2, 6},
	{6, 1, 2, 3, 0, 4, 5, 9, 7, 8},
	{3, 6, 7, 4, 2, 0, 9, 5, 8, 1},
	{5, 8, 6, 9, 7, 2, 0, 1, 3, 4},
	{8, 9, 4, 5, 3, 6, 2, 0, 1, 7},
	{9, 4, 3, 8, 6, 1, 7, 2, 0, 5},
	{2, 5, 8, 1, 4, 3, 6, 7, 9, 0},
}

// DammCheckDigit returns the Damm check digit of a string of decimal digits.
// Appended to the digits, it catches every single wrong digit and every swap
// of two neighbouring digits.
func DammCheckDigit(digits string) (byte, error) {
	var interim byte
	for i := 0; i < len(digits); i++ {
		digit := digits[i]
		if digit < '0' || digit > '9' {
			return 0, fmt.Errorf("%q is not a decimal digit", digit)
		}
		interim = dammTable[interim][digit-'0']
	}
	return '0' + interim, nil
}

func CheckPassword(passwordHash []byte, password string) error {
	return bcrypt.CompareHashAndPassword(passwordHash, []byte(password))
}
