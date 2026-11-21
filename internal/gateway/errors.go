package gateway

import (
	"context"
	"errors"

	"demonslayer2d/internal/auth"
	"demonslayer2d/internal/pb"
	"demonslayer2d/internal/store"
	"demonslayer2d/internal/world"
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

// transferFailure says how to answer a transfer that failed before the
// character left its zone, other than by a refusal: with an error code, or
// (code 0) by closing the connection with a kick reason. A timeout means the
// old zone's inbox stayed full, so the client may try again; a stopped world
// or zone, or a character the zone no longer has, means the zone is evicting
// it and kicking the connection (only the first kick counts).
func transferFailure(err error) (pb.ErrorCode, pb.KickReason) {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return pb.ErrorCode_ERROR_CODE_RATE_LIMITED, 0
	case errors.Is(err, world.ErrStopped), errors.Is(err, zone.ErrZoneClosed), errors.Is(err, zone.ErrNotInZone):
		return 0, pb.KickReason_KICK_REASON_SERVER_SHUTDOWN
	default:
		return pb.ErrorCode_ERROR_CODE_INTERNAL, 0
	}
}

// lostKick is the kick for a character that left its zone but joined no
// other (world.Transfer returned its save): SERVER_SHUTDOWN when the world
// stopped, else ZONE_RESTART, which asks the player to log in again.
func lostKick(err error) pb.KickReason {
	if errors.Is(err, world.ErrStopped) {
		return pb.KickReason_KICK_REASON_SERVER_SHUTDOWN
	}
	return pb.KickReason_KICK_REASON_ZONE_RESTART
}
