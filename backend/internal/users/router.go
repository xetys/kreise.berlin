package users

import (
	"github.com/go-chi/chi/v5"
)

// MountAdmin mounts the authenticated admin lifecycle routes. Caller must
// apply the auth+CSRF middleware higher up.
func MountAdmin(r chi.Router, s *Service) {
	r.Get("/admin/users", s.HandleList)
	r.Post("/admin/users", s.HandleInvite)
	r.Post("/admin/users/{id}/resend-invite", s.HandleResendInvite)
	r.Post("/admin/users/{id}/deactivate", s.HandleDeactivate)
	r.Post("/admin/users/{id}/reactivate", s.HandleReactivate)
	r.Post("/admin/users/{id}/role", s.HandleRoleChange)
	r.Post("/admin/users/{id}/force-logout", s.HandleForceLogout)
	r.Post("/admin/users/{id}/set-password", s.HandleAdminSetPassword)
	r.Post("/admin/account/password", s.HandleAccountChangePassword)
	r.Patch("/admin/account/profile", s.HandleAccountUpdateProfile)
}

// MountPublic mounts the unauthenticated setup-token / reset-token routes.
func MountPublic(r chi.Router, s *Service) {
	r.Post("/setup/{token}", s.HandleSetupComplete)
	r.Post("/forgot-password", s.HandleForgotPassword)
	r.Post("/reset-password/{token}", s.HandleResetPassword)
}
