package users

import (
	"context"
	"fmt"

	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/mail"
)

// sendInviteEmail tells the new user how to complete their setup.
func (s *Service) sendInviteEmail(ctx context.Context, user db.User, rawToken string) error {
	if s.mailer == nil {
		return nil
	}
	roleDE := roleLabelDE(user.Role)
	roleEN := roleLabelEN(user.Role)
	url := s.buildSetupURL(rawToken)

	displayName := user.Email
	if user.DisplayName != nil && *user.DisplayName != "" {
		displayName = *user.DisplayName
	}

	bodyDE := fmt.Sprintf(
		"Hallo %s,\n\n"+
			"du wurdest auf kreise.berlin als %s eingeladen. Bitte vergib dein Passwort und schließe das Setup ab:\n\n"+
			"%s\n\n"+
			"Der Link ist 7 Tage gültig.",
		displayName, roleDE, url,
	)
	bodyEN := fmt.Sprintf(
		"Hi %s,\n\n"+
			"you've been invited to kreise.berlin as %s. Please set your password and finish setup:\n\n"+
			"%s\n\n"+
			"The link is valid for 7 days.",
		displayName, roleEN, url,
	)
	spec := mail.BilingualSpec{
		SubjectDE: "Einladung zur Admin-Verwaltung kreise.berlin",
		SubjectEN: "kreise.berlin admin invitation",
		BodyDE:    bodyDE,
		BodyEN:    bodyEN,
	}
	msg, err := mail.RenderBilingualThemed("admin.invite", spec, mail.Theme{}, mail.TemplateData{}, nil)
	if err != nil {
		return err
	}
	msg.To = user.Email
	return s.mailer.Send(ctx, msg)
}

func roleLabelDE(role string) string {
	switch role {
	case "global_admin":
		return "globaler Admin"
	case "event_admin":
		return "Veranstaltungs-Admin"
	case "event_manager":
		return "Veranstaltungs-Manager"
	default:
		return role
	}
}

func roleLabelEN(role string) string {
	switch role {
	case "global_admin":
		return "global admin"
	case "event_admin":
		return "event admin"
	case "event_manager":
		return "event manager"
	default:
		return role
	}
}
