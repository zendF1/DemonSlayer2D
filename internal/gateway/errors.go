package gateway

import (
	"errors"

	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/zone"
)

// errorCode maps a service error to the protocol error code; unknown errors are INTERNAL.
func errorCode(err error) pb.ErrorCode {
	switch {
	case errors.Is(err, auth.ErrUsernameInvalid):
		return pb.ErrorCode_ERROR_CODE_USERNAME_INVALID
	case errors.Is(err, auth.ErrPasswordInvalid):
		return pb.ErrorCode_ERROR_CODE_PASSWORD_INVALID
	case errors.Is(err, auth.ErrNameInvalid):
		return pb.ErrorCode_ERROR_CODE_NAME_INVALID
	case errors.Is(err, auth.ErrAuthFailed):
		return pb.ErrorCode_ERROR_CODE_AUTH_FAILED
	case errors.Is(err, auth.ErrRateLimited):
		return pb.ErrorCode_ERROR_CODE_RATE_LIMITED
	case errors.Is(err, store.ErrUsernameTaken):
		return pb.ErrorCode_ERROR_CODE_USERNAME_TAKEN
	case errors.Is(err, store.ErrNameTaken):
		return pb.ErrorCode_ERROR_CODE_NAME_TAKEN
	case errors.Is(err, store.ErrSlotOccupied):
		return pb.ErrorCode_ERROR_CODE_SLOT_OCCUPIED
	case errors.Is(err, store.ErrNotFound):
		return pb.ErrorCode_ERROR_CODE_CHAR_NOT_FOUND
	case errors.Is(err, zone.ErrZoneFull):
		return pb.ErrorCode_ERROR_CODE_ZONE_FULL
	default:
		return pb.ErrorCode_ERROR_CODE_INTERNAL
	}
}
