package liftoff

import "testing"

func TestBranchForDelete(t *testing.T) {
	cases := []struct {
		name string
		plan WashPlan
		want string
	}{
		{
			name: "uses Branch when set",
			plan: WashPlan{Name: "voice-agent", Branch: "acs/voice-agent-cleanup"},
			want: "acs/voice-agent-cleanup",
		},
		{
			name: "falls back to Name when Branch empty",
			plan: WashPlan{Name: "voice-agent"},
			want: "voice-agent",
		},
		{
			name: "Branch is identical to Name",
			plan: WashPlan{Name: "feat", Branch: "feat"},
			want: "feat",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := branchForDelete(tc.plan); got != tc.want {
				t.Fatalf("branchForDelete(%+v) = %q, want %q", tc.plan, got, tc.want)
			}
		})
	}
}
