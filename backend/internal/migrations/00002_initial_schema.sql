-- +goose Up

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================================
-- Identity & sessions
-- ============================================================================

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('global_admin', 'event_admin', 'event_manager')),
    display_name TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    disabled_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX users_email_lower_idx ON users (lower(email));

-- DB-backed sessions; cookie carries the base64-encoded id only.
CREATE TABLE sessions (
    id BYTEA PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    user_agent TEXT,
    ip INET
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id) WHERE revoked_at IS NULL;
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- ============================================================================
-- Events & assignments
-- ============================================================================

CREATE TABLE events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    banner_ref TEXT,                                -- S3 object key
    color_primary TEXT NOT NULL DEFAULT '#000000',
    color_secondary TEXT NOT NULL DEFAULT '#ffffff',
    color_text TEXT NOT NULL DEFAULT '#000000',
    location TEXT NOT NULL DEFAULT '',
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    participant_limit INTEGER,                      -- NULL = unlimited, no waitlist
    pricing_mode TEXT NOT NULL CHECK (pricing_mode IN ('matrix', 'donation')),
    currency TEXT NOT NULL DEFAULT 'EUR',
    default_locale TEXT NOT NULL DEFAULT 'de' CHECK (default_locale IN ('de', 'en')),
    is_public BOOLEAN NOT NULL DEFAULT FALSE,
    archived_at TIMESTAMPTZ,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT events_dates_valid CHECK (ends_at > starts_at),
    CONSTRAINT events_participant_limit_positive CHECK (participant_limit IS NULL OR participant_limit > 0)
);
CREATE INDEX events_public_starts_at_idx ON events (is_public, starts_at) WHERE archived_at IS NULL;

CREATE TABLE event_admins (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, event_id)
);
CREATE INDEX event_admins_event_id_idx ON event_admins (event_id);

CREATE TABLE event_managers (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, event_id)
);
CREATE INDEX event_managers_event_id_idx ON event_managers (event_id);

CREATE TABLE program_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    ordering INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX program_entries_event_starts_at_idx ON program_entries (event_id, starts_at);

-- ============================================================================
-- Pricing & coupons
-- ============================================================================

CREATE TABLE price_phases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    ordering INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT price_phases_dates_valid CHECK (ends_at > starts_at)
);
CREATE INDEX price_phases_event_id_idx ON price_phases (event_id);

CREATE TABLE price_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    ordering INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX price_categories_event_id_idx ON price_categories (event_id);

CREATE TABLE price_durations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    ordering INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX price_durations_event_id_idx ON price_durations (event_id);

CREATE TABLE prices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    phase_id UUID NOT NULL REFERENCES price_phases(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES price_categories(id) ON DELETE CASCADE,
    duration_id UUID REFERENCES price_durations(id) ON DELETE CASCADE,
    amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
    UNIQUE (phase_id, category_id, duration_id)
);
CREATE INDEX prices_event_id_idx ON prices (event_id);

CREATE TABLE donation_configs (
    event_id UUID PRIMARY KEY REFERENCES events(id) ON DELETE CASCADE,
    suggested_minor BIGINT NOT NULL DEFAULT 0,
    min_minor BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT donation_amounts_nonneg CHECK (suggested_minor >= 0 AND min_minor >= 0)
);

CREATE TABLE coupons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('fixed_reduce', 'percental_reduce', 'guestlist')),
    value_minor BIGINT,
    value_percent INTEGER,
    max_uses INTEGER,
    valid_from TIMESTAMPTZ,
    valid_to TIMESTAMPTZ,
    single_use_per_email BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_id, code),
    CONSTRAINT coupons_value_for_type CHECK (
        (type = 'fixed_reduce'    AND value_minor IS NOT NULL AND value_minor > 0 AND value_percent IS NULL) OR
        (type = 'percental_reduce' AND value_percent IS NOT NULL AND value_percent > 0 AND value_percent <= 100 AND value_minor IS NULL) OR
        (type = 'guestlist'        AND value_minor IS NULL AND value_percent IS NULL)
    )
);

CREATE TABLE coupon_phase_filters (
    coupon_id UUID NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    phase_id UUID NOT NULL REFERENCES price_phases(id) ON DELETE CASCADE,
    PRIMARY KEY (coupon_id, phase_id)
);

CREATE TABLE coupon_category_filters (
    coupon_id UUID NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES price_categories(id) ON DELETE CASCADE,
    PRIMARY KEY (coupon_id, category_id)
);

-- ============================================================================
-- Bookings, participants, tickets
-- ============================================================================

CREATE TABLE bookings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE RESTRICT,
    contact_email TEXT NOT NULL,
    contact_name TEXT NOT NULL,
    contact_phone TEXT,
    locale TEXT NOT NULL DEFAULT 'de' CHECK (locale IN ('de', 'en')),
    status TEXT NOT NULL CHECK (status IN ('booked', 'paid', 'canceled')),
    total_amount_minor BIGINT NOT NULL CHECK (total_amount_minor >= 0),
    currency TEXT NOT NULL DEFAULT 'EUR',
    coupon_id UUID REFERENCES coupons(id) ON DELETE SET NULL,
    payment_method TEXT CHECK (payment_method IN ('paypal', 'bank_transfer', 'donation') OR payment_method IS NULL),
    payment_reference TEXT,
    reservation_expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at TIMESTAMPTZ,
    canceled_at TIMESTAMPTZ
);
CREATE INDEX bookings_event_status_idx ON bookings (event_id, status);
CREATE INDEX bookings_contact_email_idx ON bookings (lower(contact_email));

CREATE TABLE coupon_redemptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    coupon_id UUID NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    redeemed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (coupon_id, booking_id)
);
CREATE INDEX coupon_redemptions_coupon_id_idx ON coupon_redemptions (coupon_id);

CREATE TABLE participants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    email TEXT,
    newsletter_optin BOOLEAN NOT NULL DEFAULT FALSE,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX participants_event_id_idx ON participants (event_id);
CREATE INDEX participants_booking_id_idx ON participants (booking_id);
CREATE INDEX participants_newsletter_email_idx ON participants (lower(email)) WHERE newsletter_optin = TRUE;

-- One ticket per participant. Pricing snapshot fields use ON DELETE SET NULL
-- because the amount lives in tickets.amount_minor; pricing config edits must
-- not break already-issued tickets.
CREATE TABLE tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    participant_id UUID NOT NULL UNIQUE REFERENCES participants(id) ON DELETE CASCADE,
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('booked', 'paid', 'canceled', 'checked_in')),
    qr_token TEXT NOT NULL UNIQUE,
    phase_id UUID REFERENCES price_phases(id) ON DELETE SET NULL,
    category_id UUID REFERENCES price_categories(id) ON DELETE SET NULL,
    duration_id UUID REFERENCES price_durations(id) ON DELETE SET NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at TIMESTAMPTZ,
    canceled_at TIMESTAMPTZ,
    checked_in_at TIMESTAMPTZ
);
CREATE INDEX tickets_event_status_idx ON tickets (event_id, status);
CREATE INDEX tickets_booking_id_idx ON tickets (booking_id);

-- ============================================================================
-- Waitlist (only used when events.participant_limit IS NOT NULL)
-- ============================================================================

CREATE TABLE waitlist_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    contact_email TEXT NOT NULL,
    contact_name TEXT NOT NULL,
    locale TEXT NOT NULL DEFAULT 'de' CHECK (locale IN ('de', 'en')),
    requested_seats INTEGER NOT NULL DEFAULT 1 CHECK (requested_seats > 0),
    selection_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    promoted_at TIMESTAMPTZ,
    claim_deadline TIMESTAMPTZ,
    fulfilled_booking_id UUID REFERENCES bookings(id) ON DELETE SET NULL
);
CREATE INDEX waitlist_event_created_idx ON waitlist_entries (event_id, created_at);

-- ============================================================================
-- Email log & audit log (preserved across event deletion)
-- ============================================================================

CREATE TABLE email_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    to_email TEXT NOT NULL,
    template TEXT NOT NULL,
    subject TEXT NOT NULL,
    locale TEXT NOT NULL CHECK (locale IN ('de', 'en')),
    status TEXT NOT NULL CHECK (status IN ('sent', 'failed')),
    error TEXT,
    ses_message_id TEXT,
    related_event_id UUID REFERENCES events(id) ON DELETE SET NULL,
    related_booking_id UUID REFERENCES bookings(id) ON DELETE SET NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX email_log_to_idx ON email_log (lower(to_email));
CREATE INDEX email_log_event_idx ON email_log (related_event_id);

CREATE TABLE audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    event_id UUID REFERENCES events(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    target_kind TEXT NOT NULL,
    target_id TEXT,
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_user_id);
CREATE INDEX audit_log_event_idx ON audit_log (event_id);
CREATE INDEX audit_log_created_idx ON audit_log (created_at);

-- +goose Down

DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS email_log;
DROP TABLE IF EXISTS waitlist_entries;
DROP TABLE IF EXISTS tickets;
DROP TABLE IF EXISTS participants;
DROP TABLE IF EXISTS coupon_redemptions;
DROP TABLE IF EXISTS bookings;
DROP TABLE IF EXISTS coupon_category_filters;
DROP TABLE IF EXISTS coupon_phase_filters;
DROP TABLE IF EXISTS coupons;
DROP TABLE IF EXISTS donation_configs;
DROP TABLE IF EXISTS prices;
DROP TABLE IF EXISTS price_durations;
DROP TABLE IF EXISTS price_categories;
DROP TABLE IF EXISTS price_phases;
DROP TABLE IF EXISTS program_entries;
DROP TABLE IF EXISTS event_managers;
DROP TABLE IF EXISTS event_admins;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
