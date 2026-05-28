package events

import (
	"github.com/go-chi/chi/v5"

	"github.com/dsteiman/tickets-general/backend/internal/authz"
)

// MountAdmin mounts the admin-side event endpoints under r. Caller is
// responsible for any session-required middleware (auth.Service.Middleware)
// applied above this mount; this function only adds the per-action
// authorization guards.
//
//	r.Group(func(r chi.Router) {
//	    r.Use(authSvc.Middleware(true))
//	    events.MountAdmin(r, eventsService, guard)
//	})
func MountAdmin(r chi.Router, s *Service, guard *authz.HTTPGuard) {
	r.Route("/admin/events", func(r chi.Router) {
		// Listing is allowed for any role with a session — the handler
		// scopes results by role.
		r.Get("/", s.HandleList)

		// Create: platform-level event_admin (or global_admin).
		r.With(guard.Require(authz.ActionEventCreate)).Post("/", s.HandleCreate)

		// Per-event subroutes.
		r.Route("/{id}", func(r chi.Router) {
			r.With(guard.RequireForEventParam(authz.ActionEventRead, "id")).Get("/", s.HandleGet)
			// Public-shaped detail bypassing is_public — admins use this to
			// preview unpublished events. Same permission as the regular Get.
			r.With(guard.RequireForEventParam(authz.ActionEventRead, "id")).Get("/preview", s.HandleAdminPreview)
			r.With(guard.RequireForEventParam(authz.ActionEventUpdate, "id")).Patch("/", s.HandleUpdate)
			r.With(guard.RequireForEventParam(authz.ActionEventArchive, "id")).Post("/archive", s.HandleArchive)
			r.With(guard.RequireForEventParam(authz.ActionEventPublish, "id")).Post("/publish", s.HandlePublish)
			r.With(guard.RequireForEventParam(authz.ActionEventPublish, "id")).Post("/unpublish", s.HandleUnpublish)

			// Banner upload uses the same permission as event update.
			r.With(guard.RequireForEventParam(authz.ActionEventUpdate, "id")).Post("/banner", s.HandleBannerUpload)

			// Program entries — same gate as updating the event.
			r.Route("/program", func(r chi.Router) {
				r.With(guard.RequireForEventParam(authz.ActionEventRead, "id")).Get("/", s.HandleListProgram)
				r.With(guard.RequireForEventParam(authz.ActionEventUpdate, "id")).Post("/", s.HandleCreateProgramEntry)
				r.With(guard.RequireForEventParam(authz.ActionEventUpdate, "id")).Patch("/{entryId}", s.HandleUpdateProgramEntry)
				r.With(guard.RequireForEventParam(authz.ActionEventUpdate, "id")).Delete("/{entryId}", s.HandleDeleteProgramEntry)
			})

			// Team management.
			r.With(guard.RequireForEventParam(authz.ActionEventRead, "id")).Get("/team", s.HandleListTeam)
			r.With(guard.RequireForEventParam(authz.ActionEventAddAdmin, "id")).Post("/admins", s.HandleAssignAdmin)
			r.With(guard.RequireForEventParam(authz.ActionEventAddAdmin, "id")).Delete("/admins/{userId}", s.HandleUnassignAdmin)
			r.With(guard.RequireForEventParam(authz.ActionEventAddManager, "id")).Post("/managers", s.HandleAssignManager)
			r.With(guard.RequireForEventParam(authz.ActionEventAddManager, "id")).Delete("/managers/{userId}", s.HandleUnassignManager)

			// Pricing config (phases × categories × durations + donation + coupons).
			r.Route("/pricing", func(r chi.Router) {
				r.With(guard.RequireForEventParam(authz.ActionEventRead, "id")).Get("/", s.HandlePricingSnapshot)

				r.With(guard.RequireForEventParam(authz.ActionPricingEdit, "id")).Put("/donation", s.HandleUpsertDonationConfig)

				r.With(guard.RequireForEventParam(authz.ActionPricingEdit, "id")).Post("/phases", s.HandleCreatePhase)
				r.With(guard.RequireForEventParam(authz.ActionPricingEdit, "id")).Delete("/phases/{phaseId}", s.HandleDeletePhase)

				r.With(guard.RequireForEventParam(authz.ActionPricingEdit, "id")).Post("/categories", s.HandleCreateCategory)
				r.With(guard.RequireForEventParam(authz.ActionPricingEdit, "id")).Delete("/categories/{catId}", s.HandleDeleteCategory)

				r.With(guard.RequireForEventParam(authz.ActionPricingEdit, "id")).Post("/durations", s.HandleCreateDuration)
				r.With(guard.RequireForEventParam(authz.ActionPricingEdit, "id")).Delete("/durations/{durId}", s.HandleDeleteDuration)

				r.With(guard.RequireForEventParam(authz.ActionPricingEdit, "id")).Put("/prices", s.HandleUpsertPrice)
				r.With(guard.RequireForEventParam(authz.ActionPricingEdit, "id")).Delete("/prices/{priceId}", s.HandleDeletePrice)

				r.With(guard.RequireForEventParam(authz.ActionPricingEdit, "id")).Post("/coupons", s.HandleCreateCoupon)
				r.With(guard.RequireForEventParam(authz.ActionPricingEdit, "id")).Delete("/coupons/{couponId}", s.HandleDeleteCoupon)
			})
		})
	})
}

// MountPublic mounts the public-only routes (no auth). Banner pass-through is
// at the root; the event listing/detail and quote/booking endpoints live
// under /api.
func MountPublic(r chi.Router, s *Service) {
	r.Get("/banners/*", s.HandleBannerStream)
}

// MountPublicAPI mounts the public /api/events routes inside the /api group
// (so they pick up the CSRF middleware — only the unsafe-method endpoints
// will actually require the header).
func MountPublicAPI(r chi.Router, s *Service) {
	r.Get("/events", s.HandlePublicList)
	r.Get("/events/{slug}", s.HandlePublicGetBySlug)
}
