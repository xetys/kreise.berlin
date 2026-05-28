package events

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/auth"
	"github.com/dsteiman/tickets-general/backend/internal/db"
	"github.com/dsteiman/tickets-general/backend/internal/logging"
)

const (
	maxBannerSize = 5 * 1024 * 1024 // 5 MiB
)

var allowedBannerTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// HandleBannerUpload: POST /api/admin/events/{id}/banner (multipart form, field "banner").
func (s *Service) HandleBannerUpload(w http.ResponseWriter, r *http.Request) {
	if s.objectstore == nil {
		writeErr(w, http.StatusServiceUnavailable, "storage_unavailable", "objectstore not configured")
		return
	}
	id, err := parseEventID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_event_id", "must be a UUID")
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no session")
		return
	}

	if err := r.ParseMultipartForm(maxBannerSize + 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_multipart", err.Error())
		return
	}
	file, header, err := r.FormFile("banner")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing_file", "form field 'banner' required")
		return
	}
	defer file.Close()

	if header.Size > maxBannerSize {
		writeErr(w, http.StatusRequestEntityTooLarge, "file_too_large", fmt.Sprintf("max %d bytes", maxBannerSize))
		return
	}
	contentType := header.Header.Get("Content-Type")
	ext, ok := allowedBannerTypes[contentType]
	if !ok {
		writeErr(w, http.StatusUnsupportedMediaType, "invalid_mime", "expected image/jpeg, image/png, image/webp, or image/gif")
		return
	}

	current, err := s.pool.Queries().GetEventByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "not_found", "event not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	suffix, err := randomSuffix(8)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	key := fmt.Sprintf("events/%s/banner-%s%s", id, suffix, ext)

	if err := s.objectstore.Put(r.Context(), key, file, contentType); err != nil {
		writeErr(w, http.StatusInternalServerError, "storage_put_failed", err.Error())
		return
	}

	if err := s.pool.Queries().SetEventBanner(r.Context(), s.setBannerParams(id, key)); err != nil {
		// Best-effort cleanup of the just-uploaded object on DB failure.
		_ = s.objectstore.Delete(r.Context(), key)
		writeErr(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Best-effort delete of the previous banner.
	if current.BannerRef != nil && *current.BannerRef != "" && *current.BannerRef != key {
		if err := s.objectstore.Delete(r.Context(), *current.BannerRef); err != nil {
			logging.FromContext(r.Context()).Warn("delete old banner failed", "key", *current.BannerRef, "err", err)
		}
	}

	s.recordEventAudit(r.Context(), user.ID, id, "event.banner_uploaded", map[string]any{"key": key})

	row, _ := s.pool.Queries().GetEventByID(r.Context(), id)
	writeJSON(w, http.StatusOK, toEventResponse(row))
}

// HandleBannerStream: GET /banners/{slug} — public pass-through with caching.
func (s *Service) HandleBannerStream(w http.ResponseWriter, r *http.Request) {
	if s.objectstore == nil {
		writeErr(w, http.StatusServiceUnavailable, "storage_unavailable", "objectstore not configured")
		return
	}
	slug := strings.TrimPrefix(r.URL.Path, "/banners/")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	event, err := s.pool.Queries().GetEventBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if event.BannerRef == nil || *event.BannerRef == "" {
		http.NotFound(w, r)
		return
	}

	body, contentType, err := s.objectstore.Get(r.Context(), *event.BannerRef)
	if err != nil {
		http.Error(w, "banner unavailable", http.StatusInternalServerError)
		return
	}
	defer body.Close()

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

func (s *Service) setBannerParams(eventID uuid.UUID, key string) db.SetEventBannerParams {
	k := key
	return db.SetEventBannerParams{ID: eventID, BannerRef: &k}
}

func randomSuffix(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
