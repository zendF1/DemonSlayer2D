package gateway

import (
	"errors"
	"fmt"
	"testing"

	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/zone"
)

func TestErrorCode(t *testing.T) {
	cases := map[error]pb.ErrorCode{
		auth.ErrUsernameInvalid: pb.ErrorCode_ERROR_CODE_USERNAME_INVALID,
		auth.ErrPasswordInvalid: pb.ErrorCode_ERROR_CODE_PASSWORD_INVALID,
		auth.ErrNameInvalid:     pb.ErrorCode_ERROR_CODE_NAME_INVALID,
		auth.ErrAuthFailed:      pb.ErrorCode_ERROR_CODE_AUTH_FAILED,
		auth.ErrRateLimited:     pb.ErrorCode_ERROR_CODE_RATE_LIMITED,
		store.ErrUsernameTaken:  pb.ErrorCode_ERROR_CODE_USERNAME_TAKEN,
		store.ErrNameTaken:      pb.ErrorCode_ERROR_CODE_NAME_TAKEN,
		store.ErrSlotOccupied:   pb.ErrorCode_ERROR_CODE_SLOT_OCCUPIED,
		store.ErrNotFound:       pb.ErrorCode_ERROR_CODE_CHAR_NOT_FOUND,
		zone.ErrZoneFull:        pb.ErrorCode_ERROR_CODE_ZONE_FULL,
		errors.New("boom"):      pb.ErrorCode_ERROR_CODE_INTERNAL,
	}
	for err, want := range cases {
		if got := errorCode(fmt.Errorf("wrapped: %w", err)); got != want {
			t.Errorf("errorCode(%v) = %v, want %v", err, got, want)
		}
	}
}
