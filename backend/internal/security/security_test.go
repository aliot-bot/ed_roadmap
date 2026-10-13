package security

import "testing"

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("a long safe test password")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "a long safe test password") {
		t.Fatal("correct password rejected")
	}
	if CheckPassword(hash, "different safe password") {
		t.Fatal("wrong password accepted")
	}
	if CheckPassword("invalid", "a long safe test password") {
		t.Fatal("malformed hash accepted")
	}
}

func TestTokenHashAndRandomToken(t *testing.T) {
	a, err := RandomToken("test_")
	if err != nil {
		t.Fatal(err)
	}
	b, err := RandomToken("test_")
	if err != nil {
		t.Fatal(err)
	}
	if a == b || !EqualToken(a, TokenHash(a)) || EqualToken(a, TokenHash(b)) {
		t.Fatal("token properties failed")
	}
}

func TestAccessKeyHashUsesPepper(t *testing.T) {
	a := AccessKeyHash("secret", "pepper")
	if len(a) != 32 || string(a) == string(AccessKeyHash("secret", "other")) {
		t.Fatal("access-key hash does not depend on pepper")
	}
}
