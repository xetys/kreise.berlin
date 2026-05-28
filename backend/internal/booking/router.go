package booking

import (
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/time/rate"
)

// MountPublic mounts /api/quote and /api/bookings on the public /api router.
// Caller is responsible for the CSRF middleware higher up.
//
// /api/bookings carries a per-IP rate limiter (5 req/min, burst 5) since it
// hits the DB and sends mail.
func MountPublic(r chi.Router, s *Service) {
	limiter := NewIPRateLimiter(rate.Every(12*time.Second), 5) // 5/min sustained, burst 5
	limiter.StartGC()

	r.Post("/quote", s.HandleQuote)

	r.Group(func(r chi.Router) {
		r.Use(limiter.Middleware)
		r.Post("/bookings", s.HandleBooking)
	})

	// Waitlist self-service. Both endpoints accept a signed HMAC token in
	// the URL; no session required. Claim is rate-limited along with the
	// booking endpoint since it creates a real booking.
	r.Group(func(r chi.Router) {
		r.Use(limiter.Middleware)
		r.Post("/waitlist/claim/{token}", s.HandleWaitlistClaim)
	})
	r.Post("/waitlist/remove/{token}", s.HandleWaitlistRemove)
}

// MountAdmin mounts the authenticated admin booking surface. Caller must
// apply session middleware higher up.
//
//   - GET  /admin/events/{id}/bookings — list (per-event scope)
//   - POST /admin/bookings/{id}/mark-paid — flip booked → paid + Stage 2 email
//
// RBAC is enforced inside the handlers; the list endpoint should be wrapped
// by the caller in an event-scoped guard.
func MountAdmin(r chi.Router, s *Service) {
	r.Get("/admin/events/{id}/bookings", s.HandleListBookings)
	r.Get("/admin/events/{id}/bookings/export.csv", s.HandleBookingsCSV)
	r.Post("/admin/events/{id}/bookings/purge-test", s.HandlePurgeTestBookings)
	r.Get("/admin/events/{id}/waitlist", s.HandleListWaitlist)
	r.Get("/admin/events/{id}/check-ins", s.HandleCheckInStatus)
	r.Post("/admin/events/{id}/scan", s.HandleScan)
	r.Post("/admin/events/{id}/scan/manual", s.HandleScanManual)
	r.Post("/admin/events/{id}/scan/undo", s.HandleUndoCheckIn)
	r.Post("/admin/events/{id}/waitlist/promote", s.HandleWaitlistPromote)
	r.Post("/admin/waitlist/{id}/promote", s.HandleAdminWaitlistPromote)
	r.Post("/admin/waitlist/{id}/remove", s.HandleAdminWaitlistRemove)

	r.Post("/admin/bookings/bulk-mark-paid", s.HandleBulkMarkPaid)
	r.Post("/admin/bookings/{id}/mark-paid", s.HandleMarkPaid)
	r.Post("/admin/bookings/{id}/refund", s.HandleRefund)
	r.Post("/admin/bookings/{id}/resend-confirmation", s.HandleResendConfirmation)
	r.Patch("/admin/bookings/{id}", s.HandleEditBooking)
}
