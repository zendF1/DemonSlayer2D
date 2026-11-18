package auth

import (
	"errors"
	"strings"
	"testing"
)

var fastParams = Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse", fastParams)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=8192,t=1,p=1$") {
		t.Fatalf("unexpected encoding %q", h)
	}
	ok, err := VerifyPassword("correct horse", h)
	if err != nil || !ok {
		t.Fatalf("verify correct password: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong horse", h)
	if err != nil || ok {
		t.Fatalf("verify wrong password: ok=%v err=%v", ok, err)
	}
}

func TestHashUsesRandomSalt(t *testing.T) {
	a, _ := HashPassword("same", fastParams)
	b, _ := HashPassword("same", fastParams)
	if a == b {
		t.Fatal("two hashes of the same password are identical")
	}
}

func TestVerifyMalformed(t *testing.T) {
	for _, h := range []string{"", "plain", "$argon2i$v=19$m=1,t=1,p=1$AAAA$AAAA", "$argon2id$v=19$m=x$AAAA$AAAA", "$argon2id$v=19$m=8,t=1,p=1$!!$AAAA"} {
		if _, err := VerifyPassword("x", h); !errors.Is(err, ErrBadHash) {
			t.Errorf("VerifyPassword(%q) err = %v, want ErrBadHash", h, err)
		}
	}
}
