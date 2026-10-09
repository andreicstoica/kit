package liftoff

import (
	"encoding/json"
	"fmt"
	"strings"
)

type RexProgramStatus struct {
	Records []RexProgramRecord `json:"records"`
}

type RexProgramRecord struct {
	State        string `json:"state"`
	Kind         string `json:"kind"`
	App          string `json:"app"`
	EffectiveApp string `json:"effective_app"`
	Message      string `json:"msg"`
}

func (s *RexProgramStatus) UnmarshalJSON(data []byte) error {
	var raw struct {
		Records json.RawMessage `json:"records"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	records, err := rexArray(raw.Records)
	if err != nil {
		return err
	}
	return json.Unmarshal(records, &s.Records)
}

// ProgramStatusSummary uses reported OSC 7501 states, never process-name heuristics.
func (s RexSession) ProgramStatusSummary() string {
	counts := map[string]int{}
	for _, block := range s.DetachedBlocks {
		for _, record := range block.ProgramStatus.Records {
			counts[record.State]++
		}
	}
	for _, window := range s.Windows {
		for _, block := range window.Blocks {
			for _, record := range block.ProgramStatus.Records {
				counts[record.State]++
			}
		}
	}
	var parts []string
	for _, state := range []string{"blocked", "error", "done", "working", "idle"} {
		if n := counts[state]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, state))
		}
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, " · ")
}
