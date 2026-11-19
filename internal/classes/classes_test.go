package classes

import (
	"testing"

	"demonslayer2d/internal/pb"
)

func TestRoundTrip(t *testing.T) {
	for _, id := range []string{"viem", "thuy", "minh", "nham", "phong"} {
		c := ToPB(id)
		if c == pb.CharClass_CHAR_CLASS_UNSPECIFIED {
			t.Fatalf("ToPB(%q) = UNSPECIFIED", id)
		}
		back, ok := FromPB(c)
		if !ok || back != id {
			t.Fatalf("FromPB(ToPB(%q)) = %q, %v", id, back, ok)
		}
	}
}

func TestUnknown(t *testing.T) {
	if got := ToPB("lua"); got != pb.CharClass_CHAR_CLASS_UNSPECIFIED {
		t.Fatalf("ToPB(unknown) = %v", got)
	}
	if _, ok := FromPB(pb.CharClass_CHAR_CLASS_UNSPECIFIED); ok {
		t.Fatal("FromPB(UNSPECIFIED) should fail")
	}
}

// ______________________TEST IS MAGIC_____________________

func TestIsMagic(t *testing.T) {
	cases := map[pb.CharClass]bool{
		pb.CharClass_CHAR_CLASS_VIEM:  false,
		pb.CharClass_CHAR_CLASS_THUY:  true,
		pb.CharClass_CHAR_CLASS_MINH:  false,
		pb.CharClass_CHAR_CLASS_NHAM:  false,
		pb.CharClass_CHAR_CLASS_PHONG: true,
	}
	for c, want := range cases {
		if got := IsMagic(c); got != want {
			t.Errorf("IsMagic(%v) = %v, want %v", c, got, want)
		}
	}
}
