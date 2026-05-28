-- name: InsertEmailLog :exec
INSERT INTO email_log (
    to_email, template, subject, locale, status, error, ses_message_id,
    related_event_id, related_booking_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
