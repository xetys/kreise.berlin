// Command seed-dao-dance creates a published reference event modeled on the
// Dao Dance festival's pricing structure (early-bird / late-bird / last-bird ×
// adult / reduced × day / weekend / full). Idempotent: deletes any existing
// event with slug "dao-dance-2026" before inserting.
//
// Run via `make seed`. Reads DATABASE_URL from the environment.
//
// IMPORTANT: the EUR amounts in the priceMatrix table below are PLACEHOLDERS,
// derived from typical German conscious-festival pricing. The real Dao Dance
// page hides actual amounts behind a JS booking widget that we couldn't read.
// Replace the cents values once you have the real numbers, or edit them via
// the admin pricing UI after seeding.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dsteiman/tickets-general/backend/internal/config"
	"github.com/dsteiman/tickets-general/backend/internal/database"
	"github.com/dsteiman/tickets-general/backend/internal/db"
)

// Festival shape (verified from dao-dance.de/preise-tickets).
const (
	slug             = "dao-dance-2026"
	name             = "Dao Dance Festival 2026"
	description      = "Conscious dance festival at Badesee in Prignitz, 21–26 July 2026."
	location         = "Badesee, Prignitz"
	participantLimit = 400
)

var (
	festivalStart = time.Date(2026, 7, 21, 16, 0, 0, 0, time.UTC)
	festivalEnd   = time.Date(2026, 7, 26, 14, 0, 0, 0, time.UTC)

	earlyBirdStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	earlyBirdEnd   = time.Date(2026, 5, 15, 23, 59, 59, 0, time.UTC)
	lateBirdEnd    = time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)
	lastBirdEnd    = festivalStart // sales close when the festival opens
)

// Price matrix in EUR cents.
//
// Indices: priceMatrix[phaseName][categoryName][durationName] = cents.
// PLACEHOLDER VALUES — replace with the real numbers from dao-dance.de.
var priceMatrix = map[string]map[string]map[string]int64{
	"early-bird": {
		"adult":   {"day": 6500, "weekend": 14000, "full": 24000},
		"reduced": {"day": 5200, "weekend": 11200, "full": 19200},
	},
	"late-bird": {
		"adult":   {"day": 7500, "weekend": 16000, "full": 28000},
		"reduced": {"day": 6000, "weekend": 12800, "full": 22400},
	},
	"last-bird": {
		"adult":   {"day": 8500, "weekend": 18000, "full": 32000},
		"reduced": {"day": 6800, "weekend": 14400, "full": 25600},
	},
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	if cfg.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := run(ctx, pool); err != nil {
		fmt.Fprintf(os.Stderr, "seed failed: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, pool *database.Pool) error {
	q := pool.Queries()

	// 1. Find any global_admin to own the event. Fail if none.
	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM users WHERE role = 'global_admin' AND disabled_at IS NULL ORDER BY created_at LIMIT 1`,
	).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("no global_admin user exists; create one first")
		}
		return fmt.Errorf("find owner: %w", err)
	}
	fmt.Printf("owner: %s\n", ownerID)

	// 2. Wipe any existing event with this slug (cascade handles children).
	if _, err := pool.Exec(ctx, `DELETE FROM events WHERE slug = $1`, slug); err != nil {
		return fmt.Errorf("clean previous: %w", err)
	}

	// 3. Insert event + everything inside one transaction.
	var eventID uuid.UUID
	err := pool.WithTx(ctx, func(tx *db.Queries) error {
		limit := int32(participantLimit)
		ibanPtr := stringPtr("DE89 3704 0044 0532 0130 00")
		bicPtr := stringPtr("COBADEFFXXX")
		holderPtr := stringPtr("Dao Dance Festival e.V.")
		paypalPtr := stringPtr("daodance")
		event, err := tx.CreateEvent(ctx, db.CreateEventParams{
			Slug:              slug,
			Name:              name,
			Description:       description,
			ColorPrimary:      "#5E576A",
			ColorSecondary:    "#F5F1EE",
			ColorText:         "#1A1A1A",
			Location:          location,
			StartsAt:          festivalStart,
			EndsAt:            festivalEnd,
			ParticipantLimit:  &limit,
			PricingMode:       "matrix",
			Currency:          "EUR",
			DefaultLocale:     "de",
			IsPublic:          true,
			CreatedBy:         ownerID,
			PaymentTiming:     "beforehand",
			BankIban:          ibanPtr,
			BankBic:           bicPtr,
			BankAccountHolder: holderPtr,
			PaypalHandle:      paypalPtr,
			PaymentTestMode:   false,
		})
		if err != nil {
			return fmt.Errorf("create event: %w", err)
		}
		eventID = event.ID
		fmt.Printf("event: %s (%s)\n", event.ID, event.Slug)

		// Phases
		phaseIDs := map[string]uuid.UUID{}
		phases := []struct {
			name       string
			start, end time.Time
			ord        int32
		}{
			{"early-bird", earlyBirdStart, earlyBirdEnd, 0},
			{"late-bird", earlyBirdEnd, lateBirdEnd, 1},
			{"last-bird", lateBirdEnd, lastBirdEnd, 2},
		}
		for _, p := range phases {
			row, err := tx.CreatePricePhase(ctx, db.CreatePricePhaseParams{
				EventID: eventID, Name: p.name, StartsAt: p.start, EndsAt: p.end, Ordering: p.ord,
			})
			if err != nil {
				return fmt.Errorf("create phase %s: %w", p.name, err)
			}
			phaseIDs[p.name] = row.ID
		}
		fmt.Printf("phases: %d\n", len(phaseIDs))

		// Categories
		catIDs := map[string]uuid.UUID{}
		for i, name := range []string{"adult", "reduced"} {
			row, err := tx.CreatePriceCategory(ctx, db.CreatePriceCategoryParams{
				EventID: eventID, Name: name, Ordering: int32(i),
			})
			if err != nil {
				return fmt.Errorf("create category %s: %w", name, err)
			}
			catIDs[name] = row.ID
		}
		fmt.Printf("categories: %d\n", len(catIDs))

		// Durations
		durIDs := map[string]uuid.UUID{}
		for i, name := range []string{"day", "weekend", "full"} {
			row, err := tx.CreatePriceDuration(ctx, db.CreatePriceDurationParams{
				EventID: eventID, Name: name, Ordering: int32(i),
			})
			if err != nil {
				return fmt.Errorf("create duration %s: %w", name, err)
			}
			durIDs[name] = row.ID
		}
		fmt.Printf("durations: %d\n", len(durIDs))

		// Prices: walk priceMatrix and upsert each cell.
		var nPrices int
		for phaseName, byCat := range priceMatrix {
			pid, ok := phaseIDs[phaseName]
			if !ok {
				return fmt.Errorf("matrix references unknown phase %s", phaseName)
			}
			for catName, byDur := range byCat {
				cid, ok := catIDs[catName]
				if !ok {
					return fmt.Errorf("matrix references unknown category %s", catName)
				}
				for durName, cents := range byDur {
					did, ok := durIDs[durName]
					if !ok {
						return fmt.Errorf("matrix references unknown duration %s", durName)
					}
					dur := did
					if _, err := tx.UpsertPrice(ctx, db.UpsertPriceParams{
						EventID:     eventID,
						PhaseID:     pid,
						CategoryID:  cid,
						DurationID:  &dur,
						AmountMinor: cents,
					}); err != nil {
						return fmt.Errorf("upsert price %s/%s/%s: %w", phaseName, catName, durName, err)
					}
					nPrices++
				}
			}
		}
		fmt.Printf("prices: %d cells\n", nPrices)

		return nil
	})
	if err != nil {
		return err
	}

	// We don't auto-assign event_admins here; global_admin owns the event and
	// the listing query for global_admin returns it regardless.

	// Suppress unused var when q isn't referenced after the tx.
	_ = q

	fmt.Println("done.")
	return nil
}

func stringPtr(s string) *string { return &s }
