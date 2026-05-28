package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/db"
)

type PricingMode string

const (
	PricingModeMatrix   PricingMode = "matrix"
	PricingModeDonation PricingMode = "donation"
)

type PaymentTiming string

const (
	PaymentTimingBeforehand PaymentTiming = "beforehand"
	PaymentTimingAtDoor     PaymentTiming = "at_door"
)

type Event struct {
	ID               uuid.UUID
	Slug             string
	Name             string
	Description      string
	BannerRef        string // S3 object key; "" when no banner uploaded yet
	ColorPrimary     string
	ColorSecondary   string
	ColorText        string
	Location         string
	StartsAt         time.Time
	EndsAt           time.Time
	ParticipantLimit *int // nil = unlimited (no waitlist will ever be created)
	PricingMode      PricingMode
	Currency         string
	DefaultLocale    string
	IsPublic         bool
	ArchivedAt       *time.Time
	CreatedBy        uuid.UUID
	CreatedAt        time.Time

	PaymentTiming     PaymentTiming
	BankIBAN          string
	BankBIC           string
	BankAccountHolder string

	PaypalHandle    string
	PaymentTestMode bool

	WaitlistClaimWindowHours int
}

func (e Event) HasParticipantLimit() bool {
	return e.ParticipantLimit != nil
}

func (e Event) IsArchived() bool {
	return e.ArchivedAt != nil
}

func EventFromDB(e db.Event) Event {
	bannerRef := ""
	if e.BannerRef != nil {
		bannerRef = *e.BannerRef
	}
	var participantLimit *int
	if e.ParticipantLimit != nil {
		v := int(*e.ParticipantLimit)
		participantLimit = &v
	}
	return Event{
		ID:                e.ID,
		Slug:              e.Slug,
		Name:              e.Name,
		Description:       e.Description,
		BannerRef:         bannerRef,
		ColorPrimary:      e.ColorPrimary,
		ColorSecondary:    e.ColorSecondary,
		ColorText:         e.ColorText,
		Location:          e.Location,
		StartsAt:          e.StartsAt,
		EndsAt:            e.EndsAt,
		ParticipantLimit:  participantLimit,
		PricingMode:       PricingMode(e.PricingMode),
		Currency:          e.Currency,
		DefaultLocale:     e.DefaultLocale,
		IsPublic:          e.IsPublic,
		ArchivedAt:        e.ArchivedAt,
		CreatedBy:         e.CreatedBy,
		CreatedAt:         e.CreatedAt,
		PaymentTiming:     PaymentTiming(e.PaymentTiming),
		BankIBAN:          deref(e.BankIban),
		BankBIC:           deref(e.BankBic),
		BankAccountHolder: deref(e.BankAccountHolder),
		PaypalHandle:      deref(e.PaypalHandle),
		PaymentTestMode:   e.PaymentTestMode,

		WaitlistClaimWindowHours: int(e.WaitlistClaimWindowHours),
	}
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
