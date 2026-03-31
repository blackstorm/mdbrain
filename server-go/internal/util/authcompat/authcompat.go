package authcompat

import (
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/blowfish"
)

const (
	Algorithm        = "bcrypt+sha512"
	DefaultCost      = 12
	minCost          = 4
	maxCost          = 31
	saltSize         = 16
	rawHashSize      = 24
	legacyBcryptMin  = 59
	bcryptIterations = 64
)

var (
	ErrMalformedHash = errors.New("authcompat: malformed hash")
	ErrInvalidCost   = errors.New("authcompat: invalid cost")
)

// Derive generates a buddy-hashers-compatible bcrypt+sha512 hash.
func Derive(password string) (string, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("authcompat: generate salt: %w", err)
	}
	return deriveWithSalt(password, salt, DefaultCost)
}

// Verify checks whether attempt matches a buddy-hashers bcrypt+sha512 hash.
func Verify(attempt string, encoded string) (bool, error) {
	parsed, err := parse(encoded)
	if err != nil {
		return false, err
	}

	// Current buddy-hashers format stores the raw 24-byte BCrypt result.
	if len(parsed.password) == rawHashSize {
		candidate, err := deriveRawHash([]byte(attempt), parsed.salt, parsed.cost)
		if err != nil {
			return false, err
		}
		return subtle.ConstantTimeCompare(parsed.password, candidate) == 1, nil
	}

	// Legacy compatibility path from buddy-hashers.
	candidate := legacyCandidateHex([]byte(attempt), parsed.salt)
	return bcrypt.CompareHashAndPassword(parsed.password, []byte(candidate)) == nil, nil
}

// deriveWithSalt is intentionally package-private for deterministic tests.
func deriveWithSalt(password string, salt []byte, cost int) (string, error) {
	hash, err := deriveRawHash([]byte(password), salt, cost)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s$%s$%d$%s", Algorithm, hex.EncodeToString(salt), cost, hex.EncodeToString(hash)), nil
}

type parsedHash struct {
	salt     []byte
	cost     int
	password []byte
}

func parse(encoded string) (*parsedHash, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 {
		return nil, ErrMalformedHash
	}
	if parts[0] != Algorithm {
		return nil, ErrMalformedHash
	}

	salt, err := hex.DecodeString(parts[1])
	if err != nil || len(salt) != saltSize {
		return nil, ErrMalformedHash
	}

	cost, err := strconv.Atoi(parts[2])
	if err != nil || cost < minCost || cost > maxCost {
		return nil, ErrInvalidCost
	}

	password, err := hex.DecodeString(parts[3])
	if err != nil || len(password) == 0 {
		return nil, ErrMalformedHash
	}

	return &parsedHash{
		salt:     salt,
		cost:     cost,
		password: password,
	}, nil
}

func deriveRawHash(password []byte, salt []byte, cost int) ([]byte, error) {
	if len(salt) != saltSize {
		return nil, ErrMalformedHash
	}
	if cost < minCost || cost > maxCost {
		return nil, ErrInvalidCost
	}

	keyHash := sha512.Sum512(password)

	cipherData := make([]byte, len(magicCipherData))
	copy(cipherData, magicCipherData)

	c, err := expensiveBlowfishSetupRawSalt(keyHash[:], uint32(cost), salt)
	if err != nil {
		return nil, fmt.Errorf("authcompat: blowfish setup: %w", err)
	}

	for i := 0; i < len(cipherData); i += 8 {
		for j := 0; j < bcryptIterations; j++ {
			c.Encrypt(cipherData[i:i+8], cipherData[i:i+8])
		}
	}

	return cipherData, nil
}

func expensiveBlowfishSetupRawSalt(key []byte, cost uint32, salt []byte) (*blowfish.Cipher, error) {
	ckey := append([]byte(nil), key...)

	c, err := blowfish.NewSaltedCipher(ckey, salt)
	if err != nil {
		return nil, err
	}

	rounds := uint64(1) << cost
	for i := uint64(0); i < rounds; i++ {
		blowfish.ExpandKey(ckey, c)
		blowfish.ExpandKey(salt, c)
	}

	return c, nil
}

func legacyCandidateHex(password []byte, salt []byte) string {
	merged := make([]byte, 0, len(password)+len(salt))
	merged = append(merged, password...)
	merged = append(merged, salt...)
	sum := sha512.Sum512(merged)
	return hex.EncodeToString(sum[:])
}

var magicCipherData = []byte{
	0x4f, 0x72, 0x70, 0x68,
	0x65, 0x61, 0x6e, 0x42,
	0x65, 0x68, 0x6f, 0x6c,
	0x64, 0x65, 0x72, 0x53,
	0x63, 0x72, 0x79, 0x44,
	0x6f, 0x75, 0x62, 0x74,
}
