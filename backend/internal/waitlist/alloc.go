// Package waitlist owns the seat-allocation algorithm and the promotion
// orchestration that runs whenever capacity opens on a limited event.
//
// The allocation function in this file is pure (no DB, no side effects) so
// it can be exhaustively unit-tested. The orchestration that wraps it lives
// in service.go.
package waitlist

import "github.com/google/uuid"

// Waiter is the minimal projection of a waitlist row needed for allocation.
type Waiter struct {
	ID             uuid.UUID
	RequestedSeats int
}

// Promotion is one item of the allocation result: the waitlist row to promote
// and how many seats they consume.
type Promotion struct {
	WaiterID uuid.UUID
	Seats    int
}

// AllocateFIFO walks the waiter queue in FIFO order (caller passes them
// already ordered by created_at ASC) and returns the set of waiters to
// promote.
//
// Rule: skip-if-doesn't-fit. If a waiter's RequestedSeats exceeds the
// remaining capacity at their turn, they're skipped (stay on the waitlist)
// and the loop continues — a later waiter wanting fewer seats can still be
// served by the remaining capacity. The waiter is *not* moved in the queue;
// the next promotion attempt will see them at their original position.
//
// remaining is the number of seats actually free right now (capacity minus
// active bookings, computed by the caller under the event-row lock).
//
// Stops when remaining reaches 0 or the queue is exhausted. Waiters with
// RequestedSeats <= 0 are skipped defensively (the schema CHECK forbids
// this, but an invariant is cheap).
func AllocateFIFO(remaining int, waiters []Waiter) []Promotion {
	if remaining <= 0 || len(waiters) == 0 {
		return nil
	}
	out := make([]Promotion, 0, len(waiters))
	for _, w := range waiters {
		if remaining == 0 {
			break
		}
		if w.RequestedSeats <= 0 {
			continue
		}
		if w.RequestedSeats > remaining {
			continue
		}
		out = append(out, Promotion{WaiterID: w.ID, Seats: w.RequestedSeats})
		remaining -= w.RequestedSeats
	}
	return out
}
