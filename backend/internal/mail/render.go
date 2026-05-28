package mail

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html"
	"mime/quotedprintable"
	"strings"
	"time"
)

// Theme is the per-event color set used by the HTML email template.
type Theme struct {
	PrimaryColor   string // event.color_primary
	SecondaryColor string // event.color_secondary
	TextColor      string // event.color_text
	EventName      string
	HeaderTagline  string // e.g. dates + location, single line
}

// RenderBilingualThemed produces a Message with a styled HTML body wrapping
// the German + English bodies, themed with the event's color set. The text
// part is the bilingual plain-text from the spec; the HTML part is the same
// content wrapped in event-styled markup. Attachments (e.g. QR PNGs) are
// passed through to msg.Attachments.
//
// HTMLBody references attachments via `cid:<ContentID>` — the SES driver
// switches to a raw MIME send when attachments are present.
func RenderBilingualThemed(
	templateName string,
	spec BilingualSpec,
	theme Theme,
	data TemplateData,
	attachments []Attachment,
) (Message, error) {
	subDE, err := renderTpl("subject_de", spec.SubjectDE, data)
	if err != nil {
		return Message{}, err
	}
	subEN, err := renderTpl("subject_en", spec.SubjectEN, data)
	if err != nil {
		return Message{}, err
	}
	bodyDE, err := renderTpl("body_de", spec.BodyDE, data)
	if err != nil {
		return Message{}, err
	}
	bodyEN, err := renderTpl("body_en", spec.BodyEN, data)
	if err != nil {
		return Message{}, err
	}

	subject := subDE + " / " + subEN
	textBody := fmt.Sprintf("%s\n\n———\n\n%s\n", bodyDE, bodyEN)
	htmlBody := renderThemedHTML(theme, bodyDE, bodyEN)

	return Message{
		Subject:     subject,
		TextBody:    textBody,
		HTMLBody:    htmlBody,
		Attachments: attachments,
		Template:    templateName,
	}, nil
}

// renderThemedHTML wraps the bilingual bodies in a styled card with the
// event's hero colors. Uses inline styles only (email-client safe).
//
// Bodies preserve `cid:`-referenced inline image tags verbatim, so callers
// that want a QR can include `<img src="cid:qr-1@tickets">` in the body
// after their text.
func renderThemedHTML(theme Theme, bodyDE, bodyEN string) string {
	primary := nonEmpty(theme.PrimaryColor, "#5E576A")
	secondary := nonEmpty(theme.SecondaryColor, "#F5F1EE")
	textColor := nonEmpty(theme.TextColor, "#1A1A1A")

	var sb strings.Builder
	sb.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"></head>`)
	sb.WriteString(fmt.Sprintf(`<body style="margin:0;padding:0;background:%s;font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;line-height:1.55;color:#1a1a1a">`, html.EscapeString(secondary)))
	sb.WriteString(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%" style="background:` + html.EscapeString(secondary) + `"><tr><td align="center" style="padding:24px 12px">`)
	sb.WriteString(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="600" style="max-width:600px;background:#ffffff;border-radius:8px;overflow:hidden;box-shadow:0 1px 4px rgba(0,0,0,0.06)">`)

	// Header
	sb.WriteString(fmt.Sprintf(
		`<tr><td style="background:%s;color:%s;padding:28px 32px"><h1 style="margin:0;font-size:22px;font-weight:600">%s</h1>`,
		html.EscapeString(primary),
		html.EscapeString(textColor),
		html.EscapeString(theme.EventName),
	))
	if theme.HeaderTagline != "" {
		sb.WriteString(fmt.Sprintf(
			`<p style="margin:6px 0 0;font-size:14px;opacity:0.9">%s</p>`,
			html.EscapeString(theme.HeaderTagline),
		))
	}
	sb.WriteString(`</td></tr>`)

	// Body — German block, separator, English block.
	sb.WriteString(`<tr><td style="padding:28px 32px;font-size:15px">`)
	sb.WriteString(textToHTML(bodyDE))
	sb.WriteString(`<hr style="margin:28px 0;border:none;border-top:1px solid #eee">`)
	sb.WriteString(textToHTML(bodyEN))
	sb.WriteString(`</td></tr>`)

	// Footer
	sb.WriteString(`<tr><td style="background:#fafafa;color:#888;font-size:11px;padding:16px 32px;text-align:center">kreise.berlin</td></tr>`)
	sb.WriteString(`</table></td></tr></table></body></html>`)

	return sb.String()
}

// textToHTML converts a plain-text block to HTML preserving paragraph breaks
// and `cid:`-referenced inline images. Strings of the form
// "<<cid:qr-N@tickets>>" are replaced by an <img> tag pointing at the
// corresponding inline attachment. Marker uses `<<…>>` rather than the
// double-curly form so it doesn't collide with text/template's `{{…}}`.
func textToHTML(text string) string {
	var sb strings.Builder
	for _, paragraph := range splitParagraphs(text) {
		if paragraph == "" {
			continue
		}
		// Detect a paragraph that's just a CID image directive — render as
		// a centered inline image without surrounding <p>.
		if strings.HasPrefix(paragraph, "<<cid:") && strings.HasSuffix(paragraph, ">>") {
			cid := strings.TrimSuffix(strings.TrimPrefix(paragraph, "<<cid:"), ">>")
			fmt.Fprintf(&sb,
				`<div style="text-align:center;margin:18px 0"><img src="cid:%s" alt="QR" style="max-width:240px;height:auto;background:white;padding:8px;border-radius:6px"/></div>`,
				html.EscapeString(cid),
			)
			continue
		}
		sb.WriteString(`<p style="margin:0 0 14px">`)
		// Escape, then convert markers, then handle line breaks.
		escaped := html.EscapeString(paragraph)
		escaped = replaceCIDMarkers(escaped)
		escaped = strings.ReplaceAll(escaped, "\n", "<br>")
		sb.WriteString(escaped)
		sb.WriteString(`</p>`)
	}
	return sb.String()
}

// replaceCIDMarkers swaps `<<cid:xxx>>` text (already HTML-escaped, so it
// reads as &lt;&lt;cid:xxx&gt;&gt;) with an HTML img tag. We work in the
// escaped form so we don't have to double-escape user content around it.
func replaceCIDMarkers(s string) string {
	const open = "&lt;&lt;cid:"
	const close = "&gt;&gt;"
	for {
		i := strings.Index(s, open)
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], close)
		if j < 0 {
			return s
		}
		j += i
		cid := s[i+len(open) : j]
		img := fmt.Sprintf(`<img src="cid:%s" alt="QR" style="max-width:240px;height:auto;background:white;padding:8px;border-radius:6px;display:block;margin:8px auto"/>`, html.EscapeString(cid))
		s = s[:i] + img + s[j+len(close):]
	}
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// ------------------------------------------------------------
// MIME assembly for SES Raw send
// ------------------------------------------------------------

// MIMEOptions carries deliverability-relevant headers that the caller (the
// SES driver) supplies from configuration: a friendly From display name and
// the unsubscribe address used for the List-Unsubscribe headers.
type MIMEOptions struct {
	FromDisplayName  string // e.g. "kreise.berlin"; "" → bare address
	FromAddress      string // raw email, e.g. noreply@psychedelic-rock.com
	UnsubscribeEmail string // e.g. unsubscribe@psychedelic-rock.com; "" disables the header
}

// buildRawMIME assembles a Gmail-friendly multipart MIME message:
//
//	multipart/mixed
//	└── multipart/alternative
//	    ├── text/plain                       (quoted-printable)
//	    └── multipart/related
//	        ├── text/html                    (quoted-printable)
//	        └── image/png inline attachments (base64, Content-ID)
//
// Deliverability-conscious choices:
//   - 3-deep nested structure (mixed > alternative > [plain, related>html+images])
//     matches the canonical shape Gmail/Apple Mail/Outlook all parse cleanly.
//   - Random boundaries — no collision risk; less predictable to filters.
//   - Explicit Date and Message-ID with the From-domain so DKIM/DMARC line up.
//   - quoted-printable for text — preserves UTF-8 (umlauts) without raw 8bit.
//   - base64 for image parts.
//   - List-Unsubscribe + List-Unsubscribe-Post headers (Gmail prefers them).
func buildRawMIME(opts MIMEOptions, to, subject, textBody, htmlBody string, attachments []Attachment) []byte {
	mixedBoundary := randomBoundary("mix")
	altBoundary := randomBoundary("alt")
	relBoundary := randomBoundary("rel")

	var sb strings.Builder

	// Headers
	fmt.Fprintf(&sb, "From: %s\r\n", formatFromHeader(opts))
	fmt.Fprintf(&sb, "To: %s\r\n", to)
	fmt.Fprintf(&sb, "Subject: %s\r\n", encodeHeader(subject))
	fmt.Fprintf(&sb, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&sb, "Message-ID: <%s>\r\n", newMessageID(opts.FromAddress))
	sb.WriteString("MIME-Version: 1.0\r\n")
	if opts.UnsubscribeEmail != "" {
		fmt.Fprintf(&sb, "List-Unsubscribe: <mailto:%s?subject=unsubscribe>\r\n", opts.UnsubscribeEmail)
		sb.WriteString("List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n")
	}
	fmt.Fprintf(&sb, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", mixedBoundary)

	// --- multipart/mixed > multipart/alternative ---
	fmt.Fprintf(&sb, "--%s\r\n", mixedBoundary)
	fmt.Fprintf(&sb, "Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", altBoundary)

	// --- alternative > text/plain ---
	fmt.Fprintf(&sb, "--%s\r\n", altBoundary)
	sb.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	sb.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	sb.WriteString(quotedPrintableEncode(textBody))
	sb.WriteString("\r\n")

	// --- alternative > multipart/related (html + inline images) ---
	fmt.Fprintf(&sb, "--%s\r\n", altBoundary)
	fmt.Fprintf(&sb, "Content-Type: multipart/related; boundary=\"%s\"\r\n\r\n", relBoundary)

	fmt.Fprintf(&sb, "--%s\r\n", relBoundary)
	sb.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	sb.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	sb.WriteString(quotedPrintableEncode(htmlBody))
	sb.WriteString("\r\n")

	for _, att := range attachments {
		fmt.Fprintf(&sb, "--%s\r\n", relBoundary)
		fmt.Fprintf(&sb, "Content-Type: %s\r\n", att.ContentType)
		fmt.Fprintf(&sb, "Content-ID: <%s>\r\n", att.ContentID)
		filename := att.Filename
		if filename == "" {
			filename = att.ContentID
		}
		fmt.Fprintf(&sb, "Content-Disposition: inline; filename=\"%s\"\r\n", filename)
		sb.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		sb.WriteString(wrapBase64(att.Data))
		sb.WriteString("\r\n")
	}
	fmt.Fprintf(&sb, "--%s--\r\n", relBoundary)

	// Close alternative + mixed.
	fmt.Fprintf(&sb, "--%s--\r\n", altBoundary)
	fmt.Fprintf(&sb, "--%s--\r\n", mixedBoundary)
	return []byte(sb.String())
}

func formatFromHeader(opts MIMEOptions) string {
	if opts.FromDisplayName == "" {
		return opts.FromAddress
	}
	// Quote the display name only when it contains characters that need it.
	name := opts.FromDisplayName
	if strings.ContainsAny(name, `,;:<>@"`) {
		name = `"` + strings.ReplaceAll(name, `"`, `\"`) + `"`
	}
	return name + " <" + opts.FromAddress + ">"
}

// randomBoundary builds an unpredictable MIME boundary. The prefix is
// purely cosmetic — helps debug-trace which boundary was which.
func randomBoundary(prefix string) string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("=_tg_%s_%s", prefix, base64.RawURLEncoding.EncodeToString(b[:]))
}

// newMessageID returns a Message-ID using the domain of the From address so
// DKIM/DMARC report the same domain throughout. Falls back to a generic
// suffix when From doesn't parse cleanly.
func newMessageID(from string) string {
	domain := "tickets.local"
	if i := strings.LastIndex(from, "@"); i > 0 && i < len(from)-1 {
		end := from[i+1:]
		if j := strings.Index(end, ">"); j > 0 {
			end = end[:j]
		}
		domain = strings.TrimSpace(end)
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s.%d@%s", base64.RawURLEncoding.EncodeToString(b[:]), time.Now().UnixNano(), domain)
}

// quotedPrintableEncode wraps the standard library's quoted-printable writer
// so we can hand it a plain string and get a string back. CRLF line endings
// are preserved per RFC 2045.
func quotedPrintableEncode(s string) string {
	var buf strings.Builder
	w := quotedprintable.NewWriter(&buf)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return buf.String()
}

// encodeHeader applies RFC 2047 encoded-word for headers that may contain
// non-ASCII (e.g. German umlauts in the subject).
func encodeHeader(s string) string {
	for _, r := range s {
		if r > 127 {
			return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
		}
	}
	return s
}

// wrapBase64 base64-encodes data and inserts a CRLF every 76 chars per RFC 2045.
func wrapBase64(data []byte) string {
	enc := base64.StdEncoding.EncodeToString(data)
	var sb strings.Builder
	for i := 0; i < len(enc); i += 76 {
		end := i + 76
		if end > len(enc) {
			end = len(enc)
		}
		sb.WriteString(enc[i:end])
		sb.WriteString("\r\n")
	}
	return sb.String()
}
