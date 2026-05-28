package mail

import (
	"bytes"
	"fmt"
	"html"
	"text/template"
)

// TemplateData is the per-render variable bag.
type TemplateData map[string]any

// BilingualSpec defines the German and English content of a templated email.
// Subject and Body are text/template sources rendered with the same data
// for each language.
type BilingualSpec struct {
	SubjectDE string
	BodyDE    string
	SubjectEN string
	BodyEN    string
}

// RenderBilingual produces a Message containing German first, then English
// below a separator. Subject is "<deSubject> / <enSubject>".
//
// All system emails use this format — bookers don't pick a language.
func RenderBilingual(templateName string, spec BilingualSpec, data TemplateData) (Message, error) {
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
	htmlBody := fmt.Sprintf(
		`<html><body style="font-family:sans-serif;line-height:1.5">`+
			`<div>%s</div>`+
			`<hr style="margin:1.5em 0;border:none;border-top:1px solid #ccc"/>`+
			`<div>%s</div>`+
			`</body></html>`,
		htmlParagraphs(bodyDE),
		htmlParagraphs(bodyEN),
	)

	return Message{
		Subject:  subject,
		TextBody: textBody,
		HTMLBody: htmlBody,
		Template: templateName,
	}, nil
}

// htmlParagraphs splits text on blank lines and wraps each non-empty block in <p>.
// HTML-escapes content; preserves single linebreaks within a paragraph as <br>.
func htmlParagraphs(text string) string {
	var out bytes.Buffer
	for _, block := range splitParagraphs(text) {
		if block == "" {
			continue
		}
		out.WriteString("<p>")
		// Escape, then convert single \n inside the paragraph to <br>.
		escaped := html.EscapeString(block)
		for i, c := range escaped {
			if c == '\n' {
				out.WriteString("<br>")
				continue
			}
			out.WriteRune(c)
			_ = i
		}
		out.WriteString("</p>")
	}
	return out.String()
}

func splitParagraphs(s string) []string {
	var paras []string
	current := ""
	prevBlank := true
	for _, line := range splitLines(s) {
		if line == "" {
			if current != "" {
				paras = append(paras, current)
				current = ""
			}
			prevBlank = true
			continue
		}
		if !prevBlank && current != "" {
			current += "\n" + line
		} else {
			current = line
		}
		prevBlank = false
	}
	if current != "" {
		paras = append(paras, current)
	}
	return paras
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	} else if start == len(s) && len(s) > 0 && s[len(s)-1] == '\n' {
		lines = append(lines, "")
	}
	return lines
}

func renderTpl(name, src string, data TemplateData) (string, error) {
	t, err := template.New(name).Parse(src)
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", name, err)
	}
	return buf.String(), nil
}
