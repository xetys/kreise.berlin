// Package mail sends transactional email through Amazon SES and records every
// send in the email_log table.
//
// One transport — `aws-sdk-go-v2` SES — is used everywhere (dev included).
// Local testing requires real AWS credentials with SES access; use the SES
// sandbox + a verified sender address (or SES simulator addresses like
// success@simulator.amazonses.com) for offline iteration.
//
// All system emails are bilingual (German + English in one body); see
// RenderBilingual.
package mail

import (
	"context"

	"github.com/google/uuid"
)

// Message is what callers hand to the mailer.
//
// System emails are bilingual (German + English in one body). There is no
// per-locale rendering — RenderBilingual produces a single Message that
// contains both languages.
type Message struct {
	To       string
	Subject  string
	HTMLBody string
	TextBody string

	// Attachments are inline parts (e.g. QR PNGs referenced via cid: URLs
	// in HTMLBody). When non-empty, the SES driver switches to a raw MIME
	// send so the multipart/related shape is preserved.
	Attachments []Attachment

	// Locale is recorded in the email_log row only. Defaults to "de" when
	// empty. Not consulted by the mailer to decide content.
	Locale string

	// Template names the logical template that produced this message
	// (e.g. "booking_confirmation"). Used for diagnosis and audit, not
	// for re-rendering on the mailer side.
	Template string

	RelatedEventID   *uuid.UUID
	RelatedBookingID *uuid.UUID
}

// Attachment is an inline image (or other binary part) embedded in the email
// alongside the HTML body. Reference it from HTMLBody via `cid:<ContentID>`.
type Attachment struct {
	ContentID   string // e.g. "qr-1@tickets"
	ContentType string // e.g. "image/png"
	Filename    string // optional; shown in some clients
	Data        []byte
}

// Mailer is the interface every driver implements. Send must record the
// attempt in email_log on both success and failure.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}
