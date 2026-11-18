package auth

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestNormalizeUsername(t *testing.T) {
	ok := map[string]string{"Tanjiro": "tanjiro", "abcd": "abcd", "user_01": "user_01", strings.Repeat("a", 20): strings.Repeat("a", 20)}
	for in, want := range ok {
		got, err := NormalizeUsername(in)
		if err != nil || got != want {
			t.Errorf("NormalizeUsername(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"abc", strings.Repeat("a", 21), "has space", "dấu", "a-b-c", ""} {
		if _, err := NormalizeUsername(in); !errors.Is(err, ErrUsernameInvalid) {
			t.Errorf("NormalizeUsername(%q) err = %v, want ErrUsernameInvalid", in, err)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("12345678"); err != nil {
		t.Errorf("8 chars: %v", err)
	}
	if err := ValidatePassword(strings.Repeat("ư", 64)); err != nil {
		t.Errorf("64 runes: %v", err)
	}
	for _, p := range []string{"1234567", strings.Repeat("x", 65)} {
		if err := ValidatePassword(p); !errors.Is(err, ErrPasswordInvalid) {
			t.Errorf("ValidatePassword(len %d) err = %v", len(p), err)
		}
	}
}

func TestNormalizeCharName(t *testing.T) {
	d, k, err := NormalizeCharName("Thủy")
	if err != nil || d != "Thủy" || k != "thủy" {
		t.Fatalf("Thủy -> %q %q %v", d, k, err)
	}
	// Decomposed input (u + combining hook + ...) normalises to the same name.
	d2, k2, err := NormalizeCharName(norm.NFD.String("Thủy"))
	if err != nil || d2 != d || k2 != k {
		t.Fatalf("NFD input -> %q %q %v", d2, k2, err)
	}
	for _, ok := range []string{"Ab", "Tanjiro2", strings.Repeat("ê", 16)} {
		if _, _, err := NormalizeCharName(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"A", strings.Repeat("a", 17), "có space", "x_y", "bad!", ""} {
		if _, _, err := NormalizeCharName(bad); !errors.Is(err, ErrNameInvalid) {
			t.Errorf("%q err = %v, want ErrNameInvalid", bad, err)
		}
	}
}
