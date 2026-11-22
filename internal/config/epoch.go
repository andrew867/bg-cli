package config

import (
	"crypto/rand"
	"encoding/hex"
)

// NewCredentialEpoch returns a new non-secret history id for one installed
// OAuth grant. It is random. It is not a hash of the access token.
func NewCredentialEpoch() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
