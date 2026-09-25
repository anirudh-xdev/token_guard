package billing

import "testing"

func TestCanCoverReservation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		available int64
		amount    int64
		want      bool
	}{
		{name: "remaining covers estimate", available: 100, amount: 40, want: true},
		{name: "exact remaining", available: 40, amount: 40, want: true},
		{name: "estimate exceeds remaining", available: 10, amount: 40, want: false},
		{name: "zero estimate still blocked when empty", available: 0, amount: 0, want: false},
		{name: "zero estimate allowed when remaining", available: 5, amount: 0, want: true},
		{name: "negative remaining treated as empty", available: -2, amount: 0, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := canCoverReservation(tc.available, tc.amount); got != tc.want {
				t.Fatalf("canCoverReservation(%d, %d) = %v, want %v", tc.available, tc.amount, got, tc.want)
			}
		})
	}
}
