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
