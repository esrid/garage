package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"crypto/pbkdf2"
)

const (
	passwordAlgorithm  = "pbkdf2-sha256"
	passwordIterations = 600_000
	passwordSaltBytes  = 16
	passwordKeyBytes   = 32
)

var unknownPasswordHash = func() string {
	salt := []byte("unknown-user-salt")
	key, err := pbkdf2.Key(sha256.New, "invalid-password", salt, passwordIterations, passwordKeyBytes)
	if err != nil {
		panic(err)
	}
	return encodePasswordHash(passwordIterations, salt, key)
}()

func hashPassword(password string) (string, error) {
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, passwordKeyBytes)
	if err != nil {
		return "", err
	}
	return encodePasswordHash(passwordIterations, salt, key), nil
}

func verifyPassword(password, encoded string) (bool, error) {
	iterations, salt, expected, err := decodePasswordHash(encoded)
	if err != nil {
		return false, err
	}
	actual, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(expected))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func burnUnknownPassword(password string) error {
	if len([]byte(password)) > MaxPasswordBytes {
		password = "invalid-password"
	}
	_, err := verifyPassword(password, unknownPasswordHash)
	return err
}

func encodePasswordHash(iterations int, salt, key []byte) string {
	return fmt.Sprintf("%s$%d$%s$%s",
		passwordAlgorithm,
		iterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
}

func decodePasswordHash(encoded string) (int, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordAlgorithm {
		return 0, nil, nil, errors.New("unsupported password hash")
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations != passwordIterations {
		return 0, nil, nil, errors.New("invalid password hash work factor")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) < passwordSaltBytes {
		return 0, nil, nil, errors.New("invalid password hash salt")
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(key) != passwordKeyBytes {
		return 0, nil, nil, errors.New("invalid password hash key")
	}
	return iterations, salt, key, nil
}
