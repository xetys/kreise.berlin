package booking

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dsteiman/tickets-general/backend/internal/pricing"
)

// Domain sentinels surfaced through the JSON error code.
var (
	errEventNotFound       = errors.New("event_not_found")
	errEventEnded          = errors.New("event_ended")
	errInvalidContact      = errors.New("invalid_contact")
	errCapacityReached     = errors.New("waitlist_available") // capacity-full but waitlist would accept (Phase 7)
	errInvalidParticipants = errors.New("invalid_participants")
)

// writeBookingError maps a pricing/booking error to the canonical
// {"error": code, "developer_message": ...} JSON shape with the right HTTP
// status.
func writeBookingError(w http.ResponseWriter, err error) {
	status, code := errorToCodeAndStatus(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":             code,
		"developer_message": err.Error(),
	})
}

func errorToCodeAndStatus(err error) (int, string) {
	switch {
	case errors.Is(err, errEventNotFound):
		return http.StatusNotFound, "event_not_found"
	case errors.Is(err, errEventEnded):
		return http.StatusGone, "event_ended"
	case errors.Is(err, errInvalidContact):
		return http.StatusBadRequest, "invalid_contact"
	case errors.Is(err, errInvalidParticipants):
		return http.StatusBadRequest, "invalid_participants"
	case errors.Is(err, errCapacityReached):
		return http.StatusConflict, "waitlist_available"

	case errors.Is(err, pricing.ErrNoSelections):
		return http.StatusBadRequest, "no_selections"
	case errors.Is(err, pricing.ErrNoActivePhase):
		return http.StatusUnprocessableEntity, "no_active_phase"
	case errors.Is(err, pricing.ErrInvalidSelection):
		return http.StatusUnprocessableEntity, "invalid_selection"
	case errors.Is(err, pricing.ErrDonationBelowMin):
		return http.StatusUnprocessableEntity, "donation_below_min"
	case errors.Is(err, pricing.ErrDonationConfigMissing):
		return http.StatusUnprocessableEntity, "donation_config_missing"

	case errors.Is(err, pricing.ErrCouponNotFound):
		return http.StatusUnprocessableEntity, "coupon_not_found"
	case errors.Is(err, pricing.ErrCouponNotYetValid):
		return http.StatusUnprocessableEntity, "coupon_not_yet_valid"
	case errors.Is(err, pricing.ErrCouponExpired):
		return http.StatusUnprocessableEntity, "coupon_expired"
	case errors.Is(err, pricing.ErrCouponMaxUsesExceeded):
		return http.StatusUnprocessableEntity, "coupon_max_uses_exceeded"
	case errors.Is(err, pricing.ErrCouponAlreadyUsed):
		return http.StatusUnprocessableEntity, "coupon_already_used"
	case errors.Is(err, pricing.ErrCouponNotApplicable):
		return http.StatusUnprocessableEntity, "coupon_not_applicable"
	}
	return http.StatusInternalServerError, "internal_error"
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, code, devMsg string) {
	writeJSON(w, status, map[string]any{"error": code, "developer_message": devMsg})
}
