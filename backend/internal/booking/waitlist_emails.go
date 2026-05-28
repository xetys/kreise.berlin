package booking

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
	"github.com/dsteiman/tickets-general/backend/internal/mail"
	"github.com/dsteiman/tickets-general/backend/internal/tokens"
)

// sendWaitlistJoined — sent immediately after a submission lands on the
// waitlist. Tells the user their position and gives them a self-removal link.
func (s *Service) sendWaitlistJoined(
	ctx context.Context,
	event domain.Event,
	entry db.WaitlistEntry,
	position, total int,
) error {
	if s.mailer == nil {
		return nil
	}
	removeURL, err := s.waitlistRemoveURL(entry.ID, entry.Locale)
	if err != nil {
		return err
	}

	subjectDE := fmt.Sprintf("Du bist auf der Warteliste für %s", event.Name)
	subjectEN := fmt.Sprintf("You're on the waitlist for %s", event.Name)

	bodyDE := fmt.Sprintf(
		"Hallo %s,\n\n"+
			"Die Veranstaltung »%s« ist aktuell ausgebucht. Du stehst auf der Warteliste — Position %d von %d.\n\n"+
			"Sobald ein Platz frei wird, melden wir uns sofort. Bei kostenpflichtigen Veranstaltungen bekommst du dann einen Link, mit dem du dir den Platz innerhalb von 24 Stunden sichern kannst (zuerst klicken = zuerst bekommen). Bei Spendenveranstaltungen wirst du automatisch übernommen.\n\n"+
			"Falls du nicht mehr warten möchtest, kannst du dich jederzeit hier austragen:\n%s\n\n"+
			"Bis bald!",
		entry.ContactName, event.Name, position, total, removeURL,
	)
	bodyEN := fmt.Sprintf(
		"Hi %s,\n\n"+
			"\"%s\" is currently sold out. You're on the waitlist — position %d of %d.\n\n"+
			"We'll be in touch as soon as a spot opens. For paid events you'll get a link to claim the spot within 24 hours (first click wins). For donation events you'll be promoted automatically.\n\n"+
			"If you'd like to leave the waitlist, click here any time:\n%s\n\n"+
			"See you soon!",
		entry.ContactName, event.Name, position, total, removeURL,
	)

	spec := mail.BilingualSpec{SubjectDE: subjectDE, SubjectEN: subjectEN, BodyDE: bodyDE, BodyEN: bodyEN}
	msg, err := mail.RenderBilingualThemed("waitlist.joined", spec, themeForEvent(event), mail.TemplateData{}, nil)
	if err != nil {
		return err
	}
	msg.To = entry.ContactEmail
	msg.Locale = entry.Locale
	return s.mailer.Send(ctx, msg)
}

// sendWaitlistSpotOpened — sent to every waiter whose requested_seats fits
// in the current opening, on a paid event. Each gets a unique claim link;
// first to click wins.
func (s *Service) sendWaitlistSpotOpened(
	ctx context.Context,
	event domain.Event,
	entry db.WaitlistEntry,
	openSeats int,
) error {
	if s.mailer == nil {
		return nil
	}
	claimURL, err := s.waitlistClaimURL(entry.ID, entry.Locale)
	if err != nil {
		return err
	}
	removeURL, err := s.waitlistRemoveURL(entry.ID, entry.Locale)
	if err != nil {
		return err
	}

	hours := event.WaitlistClaimWindowHours
	if hours <= 0 {
		hours = 24
	}

	subjectDE := fmt.Sprintf("Ein Platz ist frei: %s", event.Name)
	subjectEN := fmt.Sprintf("A spot opened: %s", event.Name)

	bodyDE := fmt.Sprintf(
		"Hallo %s,\n\n"+
			"Bei »%s« ist gerade Platz frei geworden (%d Plätz%s)! Hier kannst du dir deinen Platz sichern:\n%s\n\n"+
			"Du hast %d Stunden Zeit. Wer zuerst klickt, kriegt ihn. Falls jemand schneller war, bleibst du auf der Warteliste.\n\n"+
			"Wenn du nicht mehr willst, hier austragen:\n%s",
		entry.ContactName, event.Name, openSeats, plural(openSeats, "", "e"), claimURL, hours, removeURL,
	)
	bodyEN := fmt.Sprintf(
		"Hi %s,\n\n"+
			"A spot just opened up at \"%s\" (%d seat%s)! Claim yours here:\n%s\n\n"+
			"You have %d hours. First click wins. If someone beats you, you stay on the waitlist.\n\n"+
			"Don't want it any more? Remove yourself here:\n%s",
		entry.ContactName, event.Name, openSeats, plural(openSeats, "", "s"), claimURL, hours, removeURL,
	)

	spec := mail.BilingualSpec{SubjectDE: subjectDE, SubjectEN: subjectEN, BodyDE: bodyDE, BodyEN: bodyEN}
	msg, err := mail.RenderBilingualThemed("waitlist.spot_opened", spec, themeForEvent(event), mail.TemplateData{}, nil)
	if err != nil {
		return err
	}
	msg.To = entry.ContactEmail
	msg.Locale = entry.Locale
	return s.mailer.Send(ctx, msg)
}

// sendWaitlistRemovedSelf — courtesy confirmation after self-removal.
func (s *Service) sendWaitlistRemovedSelf(
	ctx context.Context,
	event domain.Event,
	entry db.WaitlistEntry,
) error {
	if s.mailer == nil {
		return nil
	}
	subjectDE := fmt.Sprintf("Du bist von der Warteliste entfernt: %s", event.Name)
	subjectEN := fmt.Sprintf("Removed from waitlist: %s", event.Name)
	bodyDE := fmt.Sprintf(
		"Hallo %s,\n\n"+
			"du wurdest auf eigenen Wunsch von der Warteliste für »%s« entfernt. Wir melden uns nicht mehr, falls Plätze frei werden.\n\n"+
			"Bis demnächst.",
		entry.ContactName, event.Name,
	)
	bodyEN := fmt.Sprintf(
		"Hi %s,\n\n"+
			"you've been removed from the waitlist for \"%s\" at your request. We won't reach out again if spots open up.\n\n"+
			"See you another time.",
		entry.ContactName, event.Name,
	)
	spec := mail.BilingualSpec{SubjectDE: subjectDE, SubjectEN: subjectEN, BodyDE: bodyDE, BodyEN: bodyEN}
	msg, err := mail.RenderBilingualThemed("waitlist.removed_self", spec, themeForEvent(event), mail.TemplateData{}, nil)
	if err != nil {
		return err
	}
	msg.To = entry.ContactEmail
	msg.Locale = entry.Locale
	return s.mailer.Send(ctx, msg)
}

// waitlistClaimURL builds a signed claim link for a paid-event waiter.
// Token format mirrors the QR/view tokens: purpose.payload.signature, with
// the waitlist row id + a fresh random nonce as the payload.
func (s *Service) waitlistClaimURL(entryID [16]byte, locale string) (string, error) {
	tok, err := s.signWaitlistToken(tokens.PurposeWaitlistClaim, entryID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/waitlist/claim/%s",
		strings.TrimRight(s.cfg.PublicBaseURL, "/"),
		normalizeLocaleEmail(locale),
		tok,
	), nil
}

// waitlistRemoveURL builds a signed self-removal link.
func (s *Service) waitlistRemoveURL(entryID [16]byte, locale string) (string, error) {
	tok, err := s.signWaitlistToken(tokens.PurposeWaitlistRemove, entryID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/waitlist/remove/%s",
		strings.TrimRight(s.cfg.PublicBaseURL, "/"),
		normalizeLocaleEmail(locale),
		tok,
	), nil
}

func (s *Service) signWaitlistToken(purpose string, entryID [16]byte) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return tokens.Sign(purpose, entryID, nonce, s.cfg.TokenSigningKey)
}

func normalizeLocaleEmail(loc string) string {
	switch loc {
	case "en":
		return "en"
	default:
		return "de"
	}
}

func plural(n int, singular, suffix string) string {
	if n == 1 {
		return singular
	}
	return suffix
}

// themeForEvent mirrors the per-event theme construction used by the other
// transactional emails. Kept local to keep the mail package decoupled from
// the domain package.
func themeForEvent(event domain.Event) mail.Theme {
	return mail.Theme{
		PrimaryColor:   event.ColorPrimary,
		SecondaryColor: event.ColorSecondary,
		TextColor:      event.ColorText,
		EventName:      event.Name,
		HeaderTagline:  formatTagline(event),
	}
}
