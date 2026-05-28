package events

import (
	"time"

	"github.com/google/uuid"

	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/domain"
)

type EventResponse struct {
	ID               uuid.UUID `json:"id"`
	Slug             string    `json:"slug"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	BannerRef        string    `json:"banner_ref"`
	BannerURL        string    `json:"banner_url,omitempty"` // populated when slug is set
	ColorPrimary     string    `json:"color_primary"`
	ColorSecondary   string    `json:"color_secondary"`
	ColorText        string    `json:"color_text"`
	Location         string    `json:"location"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	ParticipantLimit *int      `json:"participant_limit"`
	PricingMode      string    `json:"pricing_mode"`
	Currency         string    `json:"currency"`
	DefaultLocale    string    `json:"default_locale"`
	IsPublic         bool      `json:"is_public"`
	IsArchived       bool      `json:"is_archived"`
	CreatedBy        uuid.UUID `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`

	PaymentTiming     string `json:"payment_timing"`
	BankIBAN          string `json:"bank_iban,omitempty"`
	BankBIC           string `json:"bank_bic,omitempty"`
	BankAccountHolder string `json:"bank_account_holder,omitempty"`
	PaypalHandle      string `json:"paypal_handle,omitempty"`
	PaymentTestMode   bool   `json:"payment_test_mode"`
}

func toEventResponse(e db.Event) EventResponse {
	d := domain.EventFromDB(e)
	resp := EventResponse{
		ID:                d.ID,
		Slug:              d.Slug,
		Name:              d.Name,
		Description:       d.Description,
		BannerRef:         d.BannerRef,
		ColorPrimary:      d.ColorPrimary,
		ColorSecondary:    d.ColorSecondary,
		ColorText:         d.ColorText,
		Location:          d.Location,
		StartsAt:          d.StartsAt,
		EndsAt:            d.EndsAt,
		ParticipantLimit:  d.ParticipantLimit,
		PricingMode:       string(d.PricingMode),
		Currency:          d.Currency,
		DefaultLocale:     d.DefaultLocale,
		IsPublic:          d.IsPublic,
		IsArchived:        d.IsArchived(),
		CreatedBy:         d.CreatedBy,
		CreatedAt:         d.CreatedAt,
		PaymentTiming:     string(d.PaymentTiming),
		BankIBAN:          d.BankIBAN,
		BankBIC:           d.BankBIC,
		BankAccountHolder: d.BankAccountHolder,
		PaypalHandle:      d.PaypalHandle,
		PaymentTestMode:   d.PaymentTestMode,
	}
	if d.BannerRef != "" && d.Slug != "" {
		resp.BannerURL = "/banners/" + d.Slug
	}
	return resp
}
