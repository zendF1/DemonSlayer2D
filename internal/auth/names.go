package auth

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var (
	ErrUsernameInvalid = errors.New("auth: invalid username")
	ErrPasswordInvalid = errors.New("auth: invalid password")
	ErrNameInvalid     = errors.New("auth: invalid character name")
)

// NormalizeUsername lower-cases s and checks it is 4-20 chars of a-z, 0-9, _.
func NormalizeUsername(s string) (string, error) {
	u := strings.ToLower(s)
	if len(u) < 4 || len(u) > 20 {
		return "", ErrUsernameInvalid
	}
	for i := 0; i < len(u); i++ {
		c := u[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return "", ErrUsernameInvalid
		}
	}
	return u, nil
}

// ValidatePassword checks the password is 8-64 characters.
func ValidatePassword(p string) error {
	if n := utf8.RuneCountInString(p); n < 8 || n > 64 {
		return ErrPasswordInvalid
	}
	return nil
}

// NormalizeCharName returns the NFC display name and its lower-case uniqueness key.
// A valid name is 2-16 letters or digits (Vietnamese letters allowed), no spaces or symbols.
func NormalizeCharName(s string) (display, key string, err error) {
	d := norm.NFC.String(s)
	if n := utf8.RuneCountInString(d); n < 2 || n > 16 {
		return "", "", ErrNameInvalid
	}
	for _, r := range d {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return "", "", ErrNameInvalid
		}
	}
	return d, strings.ToLower(d), nil
}
