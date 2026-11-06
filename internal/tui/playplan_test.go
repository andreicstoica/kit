package tui

import (
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestBuildPlayPlanAdopted(t *testing.T) {
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	cfg, _ := liftoff.LoadConfig()
	cfg.Worktrees["feat"] = liftoff.WorktreeMeta{Slot: 5}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	layout := liftoff.Layout{Root: t.TempDir(), Master: t.TempDir()}
	res, err := buildPlayPlan(layout, "feat", "/tmp/feat", []liftoff.Service{liftoff.SvcApp})
	if err != nil {
		t.Fatal(err)
	}
	if res.needsAdopt || res.plan.Slot != 5 {
		t.Errorf("res = %+v, want slot 5 no adopt", res)
	}
}

func TestBuildPlayPlanNeedsAdopt(t *testing.T) {
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	layout := liftoff.Layout{Root: t.TempDir(), Master: t.TempDir()}
	res, err := buildPlayPlan(layout, "unknown", "/tmp/x", []liftoff.Service{liftoff.SvcApp})
	if err != nil {
		t.Fatal(err)
	}
	if !res.needsAdopt {
		t.Errorf("want needsAdopt, got %+v", res)
	}
}

func TestBuildPlayPlanMaster(t *testing.T) {
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	layout := liftoff.Layout{Root: t.TempDir(), Master: t.TempDir()}
	res, err := buildPlayPlan(layout, "master", "/tmp/master", []liftoff.Service{liftoff.SvcApp})
	if err != nil {
		t.Fatal(err)
	}
	if res.needsAdopt || res.plan.Slot != 0 {
		t.Errorf("master res = %+v, want slot 0", res)
	}
}
