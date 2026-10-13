package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 1024 {
		return "", errors.New("password must contain 12-1024 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	h := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(h)), nil
}

func CheckPassword(encoded, password string) bool {
	p := strings.Split(encoded, "$")
	if len(p) != 6 || p[1] != "argon2id" {
		return false
	}
	var memory uint64
	var iterations uint64
	var parallelism uint64
	for _, item := range strings.Split(p[3], ",") {
		kv := strings.SplitN(item, "=", 2)
		if len(kv) != 2 {
			return false
		}
		n, err := strconv.ParseUint(kv[1], 10, 32)
		if err != nil {
			return false
		}
		switch kv[0] {
		case "m":
			memory = n
		case "t":
			iterations = n
		case "p":
			parallelism = n
		}
	}
	if memory == 0 || iterations == 0 || parallelism == 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(p[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(p[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(iterations), uint32(memory), uint8(parallelism), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func RandomToken(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}
func TokenHash(token string) []byte { sum := sha256.Sum256([]byte(token)); return sum[:] }
func AccessKeyHash(key, pepper string) []byte {
	sum := sha256.Sum256([]byte(pepper + "\x00" + key))
	return sum[:]
}
func EqualToken(raw string, hash []byte) bool {
	return subtle.ConstantTimeCompare(TokenHash(raw), hash) == 1
}
