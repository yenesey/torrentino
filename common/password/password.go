// Package password implements the PBKDF2 password hashes stored in
// settings.json. It is shared by config validation and the web login.
package password

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const (
	// DefaultIterations is the PBKDF2 work factor used by -hash-password.
	DefaultIterations = 600_000
	// MinIterations and MaxIterations bound what config validation accepts.
	MinIterations = 100_000
	MaxIterations = 2_000_000

	saltLength = 16
	keyLength  = 32
	prefix     = "pbkdf2-sha256"
)

var dummySalt = []byte("torrentino-dummy")

// Hash derives a new PBKDF2-HMAC-SHA256 hash in the
// pbkdf2-sha256$<iterations>$<salt>$<key> format.
func Hash(pw string, iterations int) (string, error) {
	if iterations < MinIterations || iterations > MaxIterations {
		return "", fmt.Errorf("iterations must be in %d..%d", MinIterations, MaxIterations)
	}
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, iterations, keyLength)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s$%d$%s$%s",
		prefix, iterations,
		base64.StdEncoding.EncodeToString(salt),
		base64.StdEncoding.EncodeToString(key),
	), nil
}

// Parse validates and decodes a stored hash.
func Parse(encoded string) (iterations int, salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != prefix {
		return 0, nil, nil, fmt.Errorf("expected %s$<iterations>$<salt>$<key>", prefix)
	}
	iterations, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, nil, nil, fmt.Errorf("invalid iteration count: %w", err)
	}
	if iterations < MinIterations || iterations > MaxIterations {
		return 0, nil, nil, fmt.Errorf("iterations must be in %d..%d, got %d", MinIterations, MaxIterations, iterations)
	}
	salt, err = base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return 0, nil, nil, fmt.Errorf("invalid salt: %w", err)
	}
	key, err = base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return 0, nil, nil, fmt.Errorf("invalid key: %w", err)
	}
	if len(salt) != saltLength {
		return 0, nil, nil, fmt.Errorf("salt must be %d bytes, got %d", saltLength, len(salt))
	}
	if len(key) != keyLength {
		return 0, nil, nil, fmt.Errorf("key must be %d bytes, got %d", keyLength, len(key))
	}
	return iterations, salt, key, nil
}

// Validate reports whether encoded is a usable hash. It does not run PBKDF2.
func Validate(encoded string) error {
	_, _, _, err := Parse(encoded)
	return err
}

// Verify checks pw against a stored hash. For an unknown user or a malformed
// hash it still performs the PBKDF2 work on a dummy value, so response timing
// does not reveal whether a username exists.
func Verify(pw, encoded string) bool {
	iterations, salt, want, err := Parse(encoded)
	if err != nil {
		dummyVerify(pw)
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iterations, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func dummyVerify(pw string) {
	_, _ = pbkdf2.Key(sha256.New, pw, dummySalt, DefaultIterations, keyLength)
}
