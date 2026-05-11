package liftoff

import "testing"

func TestAnyFailed(t *testing.T) {
	cases := []struct {
		name string
		in   []CheckResult
		want bool
	}{
		{"empty", nil, false},
		{"all ok", []CheckResult{{Status: CheckOK}, {Status: CheckOK}}, false},
		{"warn only", []CheckResult{{Status: CheckWarn}}, false},
		{"with fail", []CheckResult{{Status: CheckOK}, {Status: CheckFail}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AnyFailed(tc.in); got != tc.want {
				t.Fatalf("AnyFailed = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAnyWarned(t *testing.T) {
	if AnyWarned(nil) {
		t.Fatal("AnyWarned(nil) = true")
	}
	if AnyWarned([]CheckResult{{Status: CheckOK}}) {
		t.Fatal("AnyWarned all-ok = true")
	}
	if !AnyWarned([]CheckResult{{Status: CheckOK}, {Status: CheckWarn}}) {
		t.Fatal("AnyWarned with warn = false")
	}
}

func TestSummarize(t *testing.T) {
	in := []CheckResult{
		{Status: CheckOK},
		{Status: CheckOK},
		{Status: CheckWarn},
		{Status: CheckFail},
		{Status: CheckSkip},
	}
	s := Summarize(in)
	if s.OK != 2 || s.Warn != 1 || s.Fail != 1 || s.Skip != 1 {
		t.Fatalf("Summarize = %+v", s)
	}
}

func TestRunChecksParallelOrder(t *testing.T) {
	checks := []Check{
		{ID: "a", Run: func() CheckResult { return CheckResult{Status: CheckOK} }},
		{ID: "b", Run: func() CheckResult { return CheckResult{Status: CheckWarn} }},
		{ID: "c", Run: func() CheckResult { return CheckResult{Status: CheckFail} }},
	}
	got := RunChecks(checks)
	if len(got) != 3 {
		t.Fatalf("RunChecks len = %d", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "c" {
		t.Fatalf("RunChecks lost declaration order: %+v", got)
	}
	if got[0].Status != CheckOK || got[1].Status != CheckWarn || got[2].Status != CheckFail {
		t.Fatalf("RunChecks lost result content: %+v", got)
	}
}
