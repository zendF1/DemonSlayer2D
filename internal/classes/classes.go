// Package classes maps character class ids between the database
// (lower-case text) and the protocol enum.
package classes

import "demonslayer2d/internal/pb"

var toPB = map[string]pb.CharClass{
	"viem":  pb.CharClass_CHAR_CLASS_VIEM,
	"thuy":  pb.CharClass_CHAR_CLASS_THUY,
	"minh":  pb.CharClass_CHAR_CLASS_MINH,
	"nham":  pb.CharClass_CHAR_CLASS_NHAM,
	"phong": pb.CharClass_CHAR_CLASS_PHONG,
}

// ToPB returns the protocol enum for a database class id, or UNSPECIFIED.
func ToPB(id string) pb.CharClass { return toPB[id] }

// FromPB returns the database class id for a protocol enum.
func FromPB(c pb.CharClass) (string, bool) {
	for id, v := range toPB {
		if v == c {
			return id, true
		}
	}
	return "", false
}

// IsMagic reports whether a class is a magic class (Thuy, Phong), which gets
// less HP per level and per VIT than the physical classes (game-design 2.2).
func IsMagic(c pb.CharClass) bool {
	return c == pb.CharClass_CHAR_CLASS_THUY || c == pb.CharClass_CHAR_CLASS_PHONG
}
