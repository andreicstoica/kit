package cmd

import (
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestParseServiceList(t *testing.T) {
	tests := []struct {
		name    string
		raw     []string
		want    []liftoff.Service
		wantErr bool
	}{
		{"empty", nil, nil, false},
		{"single", []string{"app"}, []liftoff.Service{liftoff.SvcApp}, false},
		{"comma split", []string{"app,api"}, []liftoff.Service{liftoff.SvcApp, liftoff.SvcAPI}, false},
		// Repeated names must not produce duplicate entries; first-seen order wins.
		{"dedupe", []string{"app,app", "api", "app"}, []liftoff.Service{liftoff.SvcApp, liftoff.SvcAPI}, false},
		{"celery and beat", []string{"celery,beat"}, []liftoff.Service{liftoff.SvcCelery, liftoff.SvcBeat}, false},
		// admin_be aliases all resolve to the same service (and dedupe).
		{"admin_be aliases", []string{"admin_be,adminbe,admin-be"}, []liftoff.Service{liftoff.SvcAdminBE}, false},
		{"case and whitespace", []string{" APP , Admin "}, []liftoff.Service{liftoff.SvcApp, liftoff.SvcAdmin}, false},
		{"unknown service", []string{"app,nope"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseServiceList(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseServiceList(%v) = %v, want %v", tt.raw, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseServiceList(%v)[%d] = %s, want %s", tt.raw, i, got[i], tt.want[i])
				}
			}
		})
	}
}
