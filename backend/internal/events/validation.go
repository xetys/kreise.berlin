package events

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidSlug             = errors.New("invalid_slug")
	ErrSlugTaken               = errors.New("slug_taken")
	ErrInvalidName             = errors.New("invalid_name")
	ErrInvalidDates            = errors.New("invalid_dates")
	ErrInvalidColor            = errors.New("invalid_color")
	ErrInvalidParticipantLimit = errors.New("invalid_participant_limit")
	ErrInvalidPricingMode      = errors.New("invalid_pricing_mode")
	ErrInvalidCurrency         = errors.New("invalid_currency")
	ErrInvalidLocale           = errors.New("invalid_locale")
)

var (
	slugRe  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
	colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

func validateSlug(s string) error {
	if len(s) < 3 || len(s) > 80 {
		return ErrInvalidSlug
	}
	if !slugRe.MatchString(s) {
		return ErrInvalidSlug
	}
	return nil
}

func validateName(n string) error {
	n = strings.TrimSpace(n)
	if n == "" || len(n) > 200 {
		return ErrInvalidName
	}
	return nil
}

func validateColor(c string) error {
	if !colorRe.MatchString(c) {
		return ErrInvalidColor
	}
	return nil
}

func validateDates(starts, ends time.Time) error {
	if starts.IsZero() || ends.IsZero() || !ends.After(starts) {
		return ErrInvalidDates
	}
	return nil
}

func validateParticipantLimit(p *int) error {
	if p == nil {
		return nil
	}
	if *p <= 0 {
		return ErrInvalidParticipantLimit
	}
	return nil
}

func validatePricingMode(m string) error {
	switch m {
	case "matrix", "donation":
		return nil
	}
	return ErrInvalidPricingMode
}

func validateCurrency(c string) error {
	if len(c) != 3 || strings.ToUpper(c) != c {
		return ErrInvalidCurrency
	}
	return nil
}

func validateLocale(l string) error {
	switch l {
	case "de", "en":
		return nil
	}
	return ErrInvalidLocale
}

var ErrInvalidPaymentTiming = errors.New("invalid_payment_timing")

func validatePaymentTiming(t string) error {
	switch t {
	case "beforehand", "at_door":
		return nil
	}
	return ErrInvalidPaymentTiming
}
