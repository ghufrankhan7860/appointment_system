package utils

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"strconv"
	"time"
)

// TURNCredential returns short-lived TURN credentials in the scheme coturn
// checks with use-auth-secret: the username is the expiry in Unix seconds and
// name, and the password is the base64 HMAC-SHA1 of the username keyed with the
// secret shared with the TURN server.
func TURNCredential(secret string, name string, expiresAt time.Time) (string, string) {
	username := strconv.FormatInt(expiresAt.Unix(), 10) + ":" + name

	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))

	return username, base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
