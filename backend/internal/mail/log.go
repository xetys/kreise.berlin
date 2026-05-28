package mail

import (
	"context"

	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/db"
)

// localeOrDefault returns the locale field as-is when set, or the canonical
// "de" placeholder otherwise. The email_log.locale column is informational
// only — system emails are bilingual (de + en) regardless of this value.
func localeOrDefault(s string) string {
	if s == "" {
		return "de"
	}
	return s
}

func recordSent(ctx context.Context, pool *database.Pool, msg Message, sesMessageID string) error {
	var sesPtr *string
	if sesMessageID != "" {
		sesPtr = &sesMessageID
	}
	return pool.Queries().InsertEmailLog(ctx, db.InsertEmailLogParams{
		ToEmail:          msg.To,
		Template:         msg.Template,
		Subject:          msg.Subject,
		Locale:           localeOrDefault(msg.Locale),
		Status:           "sent",
		SesMessageID:     sesPtr,
		RelatedEventID:   msg.RelatedEventID,
		RelatedBookingID: msg.RelatedBookingID,
	})
}

func recordFailed(ctx context.Context, pool *database.Pool, msg Message, sendErr error) error {
	errStr := sendErr.Error()
	return pool.Queries().InsertEmailLog(ctx, db.InsertEmailLogParams{
		ToEmail:          msg.To,
		Template:         msg.Template,
		Subject:          msg.Subject,
		Locale:           localeOrDefault(msg.Locale),
		Status:           "failed",
		Error:            &errStr,
		RelatedEventID:   msg.RelatedEventID,
		RelatedBookingID: msg.RelatedBookingID,
	})
}
