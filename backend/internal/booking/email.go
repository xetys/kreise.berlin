package booking

import (
	"context"
	"fmt"
	"strings"

	"github.com/skip2/go-qrcode"

	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
	"github.com/dsteiman/tickets-general/backend/internal/mail"
	"github.com/dsteiman/tickets-general/backend/internal/tokens"
)

// sendRegistrationReceipt — Stage 1, sent at booking time when status='booked'
// (matrix + beforehand events). Lists what's reserved + payment instructions
// (bank transfer info + reference). No QR yet. Does NOT include view links —
// the holder gets them in Stage 2 once the booking is paid.
func (s *Service) sendRegistrationReceipt(
	ctx context.Context,
	event domain.Event,
	booking db.Booking,
	_ []db.Participant,
	ticketInfos []ticketInfo,
) error {
	if s.mailer == nil {
		return nil
	}

	ref := formatReference(booking.ID)
	totalDisplay := formatMoney(booking.TotalAmountMinor, booking.Currency)

	// Build the per-ticket name list (no view links in Stage 1).
	names := make([]string, 0, len(ticketInfos))
	for _, ti := range ticketInfos {
		names = append(names, ti.ParticipantName)
	}
	participantList := strings.Join(names, ", ")

	totalEUR := fmt.Sprintf("%d", booking.TotalAmountMinor/100)
	if booking.TotalAmountMinor%100 != 0 {
		totalEUR = fmt.Sprintf("%.2f", float64(booking.TotalAmountMinor)/100)
	}
	paypalURL := ""
	if event.PaypalHandle != "" {
		paypalURL = fmt.Sprintf("https://paypal.me/%s/%s%s", event.PaypalHandle, totalEUR, event.Currency)
	}

	bankInfo := ""
	if event.BankIBAN != "" {
		var bb strings.Builder
		bb.WriteString("Überweise bitte:\n")
		fmt.Fprintf(&bb, "  Empfänger: %s\n", nonEmpty(event.BankAccountHolder, "—"))
		fmt.Fprintf(&bb, "  IBAN: %s\n", event.BankIBAN)
		if event.BankBIC != "" {
			fmt.Fprintf(&bb, "  BIC: %s\n", event.BankBIC)
		}
		fmt.Fprintf(&bb, "  Betrag: %s\n", totalDisplay)
		paymentRef := derefStr(booking.PaymentReference)
		if paymentRef != "" {
			fmt.Fprintf(&bb, "  Verwendungszweck: %s\n", paymentRef)
		}
		bankInfo = bb.String()
	}
	if paypalURL != "" {
		if bankInfo != "" {
			bankInfo += "\n"
		}
		bankInfo += "Oder per PayPal:\n  " + paypalURL + "\n"
	}

	bankInfoEN := ""
	if event.BankIBAN != "" {
		var bb strings.Builder
		bb.WriteString("Please transfer to:\n")
		fmt.Fprintf(&bb, "  Account holder: %s\n", nonEmpty(event.BankAccountHolder, "—"))
		fmt.Fprintf(&bb, "  IBAN: %s\n", event.BankIBAN)
		if event.BankBIC != "" {
			fmt.Fprintf(&bb, "  BIC: %s\n", event.BankBIC)
		}
		fmt.Fprintf(&bb, "  Amount: %s\n", totalDisplay)
		paymentRef := derefStr(booking.PaymentReference)
		if paymentRef != "" {
			fmt.Fprintf(&bb, "  Reference: %s\n", paymentRef)
		}
		bankInfoEN = bb.String()
	}
	if paypalURL != "" {
		if bankInfoEN != "" {
			bankInfoEN += "\n"
		}
		bankInfoEN += "Or via PayPal:\n  " + paypalURL + "\n"
	}

	spec := mail.BilingualSpec{
		SubjectDE: fmt.Sprintf("Reservierung erhalten: %s", event.Name),
		SubjectEN: fmt.Sprintf("Reservation received: %s", event.Name),
		BodyDE: fmt.Sprintf(
			"Hallo %s,\n\n"+
				"deine Reservierung für %s ist eingegangen.\n\n"+
				"Termin: %s\n"+
				"Buchungsreferenz: %s\n"+
				"Teilnehmer: %s\n"+
				"Gesamtbetrag: %s\n\n"+
				"%s\n"+
				"Der QR-Code für dein Ticket wird per E-Mail verschickt, sobald deine Zahlung eingegangen ist. "+
				"Deine Plätze sind reserviert, bis die Zahlung bestätigt ist.\n\n"+
				"Bei Fragen einfach auf diese Mail antworten.",
			booking.ContactName,
			event.Name,
			event.StartsAt.Format("02.01.2006 15:04"),
			ref,
			participantList,
			totalDisplay,
			bankInfo,
		),
		BodyEN: fmt.Sprintf(
			"Hi %s,\n\n"+
				"your reservation for %s has been received.\n\n"+
				"When: %s\n"+
				"Booking reference: %s\n"+
				"Participants: %s\n"+
				"Total: %s\n\n"+
				"%s\n"+
				"The QR code for your ticket will be sent by email as soon as your payment is received. "+
				"Your seats are held until payment is confirmed.\n\n"+
				"Reply to this email if you have any questions.",
			booking.ContactName,
			event.Name,
			event.StartsAt.Format("Jan 2, 2006 15:04"),
			ref,
			participantList,
			totalDisplay,
			bankInfoEN,
		),
	}

	theme := mail.Theme{
		PrimaryColor:   event.ColorPrimary,
		SecondaryColor: event.ColorSecondary,
		TextColor:      event.ColorText,
		EventName:      event.Name,
		HeaderTagline:  formatTagline(event),
	}
	msg, err := mail.RenderBilingualThemed("booking.registration_receipt", spec, theme, mail.TemplateData{}, nil)
	if err != nil {
		return err
	}
	return s.dispatch(ctx, &msg, booking, event)
}

// sendTicketsConfirmed — Stage 2, sent when status flips to 'paid' (immediately
// for at_door and donation events; on mark-paid / PayPal capture for
// beforehand). Includes per-ticket inline QR + view link.
func (s *Service) sendTicketsConfirmed(
	ctx context.Context,
	event domain.Event,
	booking db.Booking,
	_ []db.Participant,
	ticketInfos []ticketInfo,
) error {
	if s.mailer == nil {
		return nil
	}

	ref := formatReference(booking.ID)
	totalDisplay := formatMoney(booking.TotalAmountMinor, booking.Currency)
	donation := event.PricingMode == domain.PricingModeDonation
	atDoor := event.PaymentTiming == domain.PaymentTimingAtDoor

	// Per-ticket sections: name + view link + inline QR placeholder.
	var deTickets, enTickets strings.Builder
	attachments := make([]mail.Attachment, 0, len(ticketInfos))
	for i, ti := range ticketInfos {
		viewToken, err := tokens.Sign(tokens.PurposeView, ti.ID, ti.Nonce, s.cfg.TokenSigningKey)
		if err != nil {
			return fmt.Errorf("sign view token: %w", err)
		}
		qrToken, err := tokens.Sign(tokens.PurposeQR, ti.ID, ti.Nonce, s.cfg.TokenSigningKey)
		if err != nil {
			return fmt.Errorf("sign qr token: %w", err)
		}
		png, err := qrcode.Encode(qrToken, qrcode.Medium, 256)
		if err != nil {
			return fmt.Errorf("render qr: %w", err)
		}
		cid := fmt.Sprintf("qr-%d@tickets", i+1)
		attachments = append(attachments, mail.Attachment{
			ContentID:   cid,
			ContentType: "image/png",
			Filename:    fmt.Sprintf("qr-%s.png", ti.ParticipantName),
			Data:        png,
		})

		viewURL := strings.TrimRight(s.cfg.PublicBaseURL, "/") + "/de/tickets/" + viewToken

		fmt.Fprintf(&deTickets, "%s — %s\n<<cid:%s>>\n\n", ti.ParticipantName, viewURL, cid)
		fmt.Fprintf(&enTickets, "%s — %s\n<<cid:%s>>\n\n", ti.ParticipantName, viewURL, cid)
	}

	subjectDE := fmt.Sprintf("Tickets bestätigt: %s", event.Name)
	subjectEN := fmt.Sprintf("Tickets confirmed: %s", event.Name)
	openingDE := "deine Tickets sind bestätigt — der QR-Code unten wird beim Eintritt gescannt."
	openingEN := "your tickets are confirmed — the QR code below is scanned at the door."
	if event.PaymentTestMode {
		subjectDE = "[TEST] " + subjectDE
		subjectEN = "[TEST] " + subjectEN
		openingDE = "[TEST-MODUS] dies ist eine Test-Buchung — keine echte Zahlung wurde ausgelöst. Ticket-Daten unten zur Vorschau."
		openingEN = "[TEST MODE] this is a test booking — no real payment was triggered. Ticket data below for preview."
	} else if donation {
		// Donation events: backend marks the booking paid immediately so the
		// QR is usable, but the booker hasn't actually transferred money yet —
		// they pay the contribution at the door. "Thanks for your donation"
		// is misleading; reframe as registration-confirmed.
		openingDE = "deine Anmeldung ist bestätigt — der QR-Code unten wird beim Eintritt gescannt. Den Beitrag entrichtest du direkt beim Event vor Ort."
		openingEN = "your registration is confirmed — the QR code below is scanned at the door. Your contribution goes straight to the organizer on site."
	} else if atDoor {
		openingDE = "deine Anmeldung ist bestätigt. Den QR-Code unten zeigst du beim Eintritt — bezahlt wird vor Ort."
		openingEN = "your registration is confirmed. Show the QR code below on arrival — payment happens on site."
	}

	atDoorNote := ""
	atDoorNoteEN := ""
	if atDoor && !donation && !event.PaymentTestMode {
		atDoorNote = "Hinweis: Die Bezahlung erfolgt direkt beim Event vor Ort.\n\n"
		atDoorNoteEN = "Note: Payment is collected on site at the event.\n\n"
	}

	// For donation events the "Gesamtbetrag" line would imply a number the
	// booker has committed to, even though they pay at the door. Reframe it
	// as a suggestion instead — or drop it entirely if the email looks
	// cleaner that way.
	totalLabelDE := "Gesamtbetrag"
	totalLabelEN := "Total"
	if donation {
		totalLabelDE = "Empfohlener Beitrag"
		totalLabelEN = "Suggested contribution"
	}

	spec := mail.BilingualSpec{
		SubjectDE: subjectDE,
		SubjectEN: subjectEN,
		BodyDE: fmt.Sprintf(
			"Hallo %s,\n\n"+
				"%s\n\n"+
				"Termin: %s\n"+
				"Buchungsreferenz: %s\n"+
				"%s: %s\n\n"+
				"%s"+
				"Tickets:\n%s"+
				"Bei Fragen einfach auf diese Mail antworten.",
			booking.ContactName,
			openingDE,
			event.StartsAt.Format("02.01.2006 15:04"),
			ref,
			totalLabelDE,
			totalDisplay,
			atDoorNote,
			deTickets.String(),
		),
		BodyEN: fmt.Sprintf(
			"Hi %s,\n\n"+
				"%s\n\n"+
				"When: %s\n"+
				"Booking reference: %s\n"+
				"%s: %s\n\n"+
				"%s"+
				"Tickets:\n%s"+
				"Reply to this email if you have any questions.",
			booking.ContactName,
			openingEN,
			event.StartsAt.Format("Jan 2, 2006 15:04"),
			ref,
			totalLabelEN,
			totalDisplay,
			atDoorNoteEN,
			enTickets.String(),
		),
	}

	theme := mail.Theme{
		PrimaryColor:   event.ColorPrimary,
		SecondaryColor: event.ColorSecondary,
		TextColor:      event.ColorText,
		EventName:      event.Name,
		HeaderTagline:  formatTagline(event),
	}
	msg, err := mail.RenderBilingualThemed("booking.tickets_confirmed", spec, theme, mail.TemplateData{}, attachments)
	if err != nil {
		return err
	}
	return s.dispatch(ctx, &msg, booking, event)
}

// dispatch fills in addressing + audit metadata on the message and sends it.
func (s *Service) dispatch(ctx context.Context, msg *mail.Message, booking db.Booking, event domain.Event) error {
	msg.To = booking.ContactEmail
	msg.Locale = booking.Locale
	eid := event.ID
	bid := booking.ID
	msg.RelatedEventID = &eid
	msg.RelatedBookingID = &bid
	return s.mailer.Send(ctx, *msg)
}

func formatTagline(e domain.Event) string {
	dates := e.StartsAt.Format("02.01.2006") + " – " + e.EndsAt.Format("02.01.2006")
	if e.Location != "" {
		return dates + " · " + e.Location
	}
	return dates
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// sendRefundNotice tells the booker their booking has been canceled and a
// refund will follow out-of-band (the platform never moves money — it's the
// admin's job via PayPal.me / wire transfer).
func (s *Service) sendRefundNotice(ctx context.Context, booking db.Booking) error {
	if s.mailer == nil {
		return nil
	}
	ref := formatReference(booking.ID)
	totalDisplay := formatMoney(booking.TotalAmountMinor, booking.Currency)
	spec := mail.BilingualSpec{
		SubjectDE: fmt.Sprintf("Stornierung: Buchung %s", ref),
		SubjectEN: fmt.Sprintf("Refund: booking %s", ref),
		BodyDE: fmt.Sprintf(
			"Hallo %s,\n\ndeine Buchung %s über %s wurde storniert. "+
				"Falls du bereits gezahlt hast, erhältst du den Betrag in Kürze zurück — der Veranstalter meldet sich.\n\n"+
				"Bei Fragen einfach auf diese Mail antworten.",
			booking.ContactName, ref, totalDisplay,
		),
		BodyEN: fmt.Sprintf(
			"Hi %s,\n\nyour booking %s for %s has been canceled. "+
				"If you've already paid, the organizer will refund you shortly via the original payment channel.\n\n"+
				"Reply to this email if you have any questions.",
			booking.ContactName, ref, totalDisplay,
		),
	}
	msg, err := mail.RenderBilingualThemed("booking.refund_notice", spec, mail.Theme{}, mail.TemplateData{}, nil)
	if err != nil {
		return err
	}
	msg.To = booking.ContactEmail
	msg.Locale = booking.Locale
	bid := booking.ID
	msg.RelatedBookingID = &bid
	return s.mailer.Send(ctx, msg)
}

func formatMoney(minor int64, currency string) string {
	whole := minor / 100
	cents := minor % 100
	if cents < 0 {
		cents = -cents
	}
	return fmt.Sprintf("%d,%02d %s", whole, cents, currency)
}
