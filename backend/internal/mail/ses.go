package mail

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/dsteiman/tickets-general/backend/internal/database"
)

type Config struct {
	Region              string
	From                string // raw email address, e.g. noreply@example.com
	FromDisplayName     string // optional friendly name shown in clients
	UnsubscribeEmail    string // optional; if set, sends List-Unsubscribe header
	ConfigurationSetARN string // optional; empty disables
}

type sesMailer struct {
	pool   *database.Pool
	client *sesv2.Client
	cfg    Config
}

// New builds the Mailer. Credentials come from the standard AWS provider
// chain (env, shared config, IAM role). Construction does not validate
// credentials — the first Send call surfaces missing/invalid creds.
func New(ctx context.Context, pool *database.Pool, cfg Config) (Mailer, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return &sesMailer{
		pool:   pool,
		client: sesv2.NewFromConfig(awsCfg),
		cfg:    cfg,
	}, nil
}

func (m *sesMailer) Send(ctx context.Context, msg Message) error {
	in := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(m.formattedFrom()),
		Destination:      &sestypes.Destination{ToAddresses: []string{msg.To}},
	}
	if m.cfg.ConfigurationSetARN != "" {
		in.ConfigurationSetName = aws.String(m.cfg.ConfigurationSetARN)
	}

	// Always use Raw send so we control headers (List-Unsubscribe, Date,
	// Message-ID, From display name). The MIME shape branches on whether
	// inline attachments are present.
	raw := buildRawMIME(MIMEOptions{
		FromDisplayName:  m.cfg.FromDisplayName,
		FromAddress:      m.cfg.From,
		UnsubscribeEmail: m.cfg.UnsubscribeEmail,
	}, msg.To, msg.Subject, msg.TextBody, msg.HTMLBody, msg.Attachments)
	in.Content = &sestypes.EmailContent{Raw: &sestypes.RawMessage{Data: raw}}

	out, err := m.client.SendEmail(ctx, in)
	if err != nil {
		_ = recordFailed(ctx, m.pool, msg, err)
		return fmt.Errorf("ses send: %w", err)
	}

	sesID := aws.ToString(out.MessageId)
	if logErr := recordSent(ctx, m.pool, msg, sesID); logErr != nil {
		return fmt.Errorf("email_log write: %w", logErr)
	}
	return nil
}

// formattedFrom matches the From header SES uses for envelope-from. SESv2
// `FromEmailAddress` accepts the friendly form `Name <addr>` directly.
func (m *sesMailer) formattedFrom() string {
	return formatFromHeader(MIMEOptions{
		FromDisplayName: m.cfg.FromDisplayName,
		FromAddress:     m.cfg.From,
	})
}
