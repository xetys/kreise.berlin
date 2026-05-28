package booking

import (
	"github.com/google/uuid"
)

// BookingRequest is the body of POST /api/bookings.
type BookingRequest struct {
	EventSlug       string             `json:"event_slug"`
	Contact         ContactInfo        `json:"contact"`
	NewsletterOptin bool               `json:"newsletter_optin"`
	CouponCode      string             `json:"coupon_code"`
	Locale          string             `json:"locale"` // "de" | "en"
	Participants    []ParticipantInput `json:"participants"`
}

// QuoteRequest is the body of POST /api/quote — same shape as BookingRequest
// minus the contact (only the email matters for single_use_per_email).
type QuoteRequest struct {
	EventSlug    string             `json:"event_slug"`
	ContactEmail string             `json:"contact_email"`
	CouponCode   string             `json:"coupon_code"`
	Participants []ParticipantInput `json:"participants"`
}

type ContactInfo struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// ParticipantInput is the per-participant payload.
//
// In matrix mode CategoryID is required and DurationID may be required
// depending on whether the event has durations. In donation mode only
// DonationAmountMinor is consulted (nil = use suggested).
type ParticipantInput struct {
	Name                string     `json:"name"`
	Email               string     `json:"email"`
	CategoryID          *uuid.UUID `json:"category_id"`
	DurationID          *uuid.UUID `json:"duration_id"`
	DonationAmountMinor *int64     `json:"donation_amount_minor"`
}

// QuoteResponse mirrors pricing.Quote with stable JSON tags.
type QuoteResponse struct {
	Currency          string             `json:"currency"`
	LineItems         []QuoteLineItem    `json:"line_items"`
	SubtotalMinor     int64              `json:"subtotal_minor"`
	DiscountMinor     int64              `json:"discount_minor"`
	TotalMinor        int64              `json:"total_minor"`
	AppliedCouponCode string             `json:"applied_coupon_code,omitempty"`
	AppliedCouponID   *uuid.UUID         `json:"applied_coupon_id,omitempty"`
	Phase             *QuotePhaseSummary `json:"phase,omitempty"`
}

type QuoteLineItem struct {
	Description string     `json:"description"`
	AmountMinor int64      `json:"amount_minor"`
	PhaseID     *uuid.UUID `json:"phase_id,omitempty"`
	CategoryID  *uuid.UUID `json:"category_id,omitempty"`
	DurationID  *uuid.UUID `json:"duration_id,omitempty"`
}

type QuotePhaseSummary struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// BookingResponse is what POST /api/bookings returns on success.
//
// Outcome discriminates two cases:
//   - "booked"     — booking + tickets created (existing flow).
//   - "waitlisted" — event was full; a waitlist_entry was created instead.
//
// Frontend reads Outcome to decide whether to navigate to /booked or
// /waitlisted. Existing fields stay populated for the "booked" outcome so
// older callers keep working.
type BookingResponse struct {
	Outcome string `json:"outcome"`

	// outcome=booked
	BookingID         uuid.UUID `json:"booking_id,omitempty"`
	BookingReference  string    `json:"booking_reference,omitempty"`
	Status            string    `json:"status,omitempty"`
	TotalAmountMinor  int64     `json:"total_amount_minor,omitempty"`
	Currency          string    `json:"currency,omitempty"`
	ParticipantCount  int       `json:"participant_count,omitempty"`
	PaymentMethod     string    `json:"payment_method,omitempty"`
	ReservationExpiry string    `json:"reservation_expires_at,omitempty"`

	// outcome=waitlisted
	WaitlistID       uuid.UUID `json:"waitlist_id,omitempty"`
	WaitlistPosition int       `json:"waitlist_position,omitempty"`
	WaitlistTotal    int       `json:"waitlist_total,omitempty"`
}
