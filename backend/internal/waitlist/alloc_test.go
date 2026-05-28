package waitlist

import (
	"testing"

	"github.com/google/uuid"
)

func mkWaiters(seats ...int) ([]Waiter, []uuid.UUID) {
	ws := make([]Waiter, len(seats))
	ids := make([]uuid.UUID, len(seats))
	for i, s := range seats {
		ids[i] = uuid.New()
		ws[i] = Waiter{ID: ids[i], RequestedSeats: s}
	}
	return ws, ids
}

func wantPromos(t *testing.T, got []Promotion, ids []uuid.UUID, expectIdx ...int) {
	t.Helper()
	if len(got) != len(expectIdx) {
		t.Fatalf("len(promotions) = %d, want %d (got=%v)", len(got), len(expectIdx), got)
	}
	for i, idx := range expectIdx {
		if got[i].WaiterID != ids[idx] {
			t.Errorf("promotion[%d] = waiter[%d] (%s), want waiter[%d] (%s)",
				i, indexOf(ids, got[i].WaiterID), got[i].WaiterID, idx, ids[idx])
		}
	}
}

func indexOf(ids []uuid.UUID, target uuid.UUID) int {
	for i, id := range ids {
		if id == target {
			return i
		}
	}
	return -1
}

func TestAllocateFIFO_FullFit(t *testing.T) {
	// 5 seats free, queue [1, 1, 2, 1] → all promoted (5 seats consumed).
	ws, ids := mkWaiters(1, 1, 2, 1)
	got := AllocateFIFO(5, ws)
	wantPromos(t, got, ids, 0, 1, 2, 3)
}

func TestAllocateFIFO_PartialSkip(t *testing.T) {
	// 3 seats free, queue [3, 2, 1] → waiter[0] takes all 3, waiter[1]
	// skipped (needs 2, 0 left), waiter[2] skipped (needs 1, 0 left).
	ws, ids := mkWaiters(3, 2, 1)
	got := AllocateFIFO(3, ws)
	wantPromos(t, got, ids, 0)
}

func TestAllocateFIFO_AllSkip(t *testing.T) {
	// 1 seat free, queue [3, 2, 5] → all skipped, none fit.
	ws, _ := mkWaiters(3, 2, 5)
	got := AllocateFIFO(1, ws)
	if len(got) != 0 {
		t.Fatalf("expected 0 promotions, got %v", got)
	}
}

func TestAllocateFIFO_EmptyQueue(t *testing.T) {
	got := AllocateFIFO(5, nil)
	if len(got) != 0 {
		t.Fatalf("expected 0 promotions, got %v", got)
	}
}

func TestAllocateFIFO_ZeroRemaining(t *testing.T) {
	ws, _ := mkWaiters(1, 1)
	got := AllocateFIFO(0, ws)
	if len(got) != 0 {
		t.Fatalf("expected 0 promotions, got %v", got)
	}
}

func TestAllocateFIFO_NegativeRemaining(t *testing.T) {
	// Defensive: caller passing a negative remaining shouldn't crash or promote.
	ws, _ := mkWaiters(1, 1)
	got := AllocateFIFO(-3, ws)
	if len(got) != 0 {
		t.Fatalf("expected 0 promotions, got %v", got)
	}
}

func TestAllocateFIFO_SkipThenPromote(t *testing.T) {
	// User-described case: waiter A wants 3, waiter B wants 1, 1 seat free.
	// A skipped (needs 3, only 1 free), B promoted (1 seat fits).
	// A stays in the queue; the test only checks one allocation step.
	ws, ids := mkWaiters(3, 1)
	got := AllocateFIFO(1, ws)
	wantPromos(t, got, ids, 1)
}

func TestAllocateFIFO_MultiSkipThenMultiPromote(t *testing.T) {
	// 4 seats free, queue [5, 5, 1, 2, 1] →
	//   waiter[0] (5) skipped, waiter[1] (5) skipped,
	//   waiter[2] (1) promoted [3 left],
	//   waiter[3] (2) promoted [1 left],
	//   waiter[4] (1) promoted [0 left].
	ws, ids := mkWaiters(5, 5, 1, 2, 1)
	got := AllocateFIFO(4, ws)
	wantPromos(t, got, ids, 2, 3, 4)
}

func TestAllocateFIFO_StopsAtZero(t *testing.T) {
	// 2 seats free, queue [1, 1, 1] → first two promoted, third skipped.
	ws, ids := mkWaiters(1, 1, 1)
	got := AllocateFIFO(2, ws)
	wantPromos(t, got, ids, 0, 1)
}

func TestAllocateFIFO_DefensiveZeroOrNegativeSeats(t *testing.T) {
	// Schema CHECK forbids requested_seats <= 0, but if it ever got past
	// the DB the allocator must not divide by zero / loop forever.
	ws, ids := mkWaiters(0, -1, 2)
	got := AllocateFIFO(5, ws)
	wantPromos(t, got, ids, 2)
}
