package liftoff

import (
	"errors"
	"fmt"
)

// An import can contain multiple Herdr workspaces for one checkout. Kit has
// one primary workspace ID, but cleanup must also close the imported secondary
// sessions. Exact, persisted checkout-to-source-to-Rex mappings authorize this;
// display labels never do. Historical IDs remain for audit and stale-map safety.
func closeImportedRexWorkspaces(path string) error {
	if path == "" {
		return nil
	}
	return withWorkspaceLock("rex:import", func() error {
		mapping, err := LoadRexImportMap()
		if err != nil {
			return err
		}
		ids := map[string]bool{}
		for source, checkout := range mapping.Checkouts {
			if checkout != "" && cleanPath(checkout) == cleanPath(path) {
				if id := mapping.Workspaces[source]; id != "" {
					ids[id] = true
				}
			}
		}
		if len(ids) == 0 {
			return nil
		}
		state, err := readRexStructure("")
		if err != nil {
			return err
		}
		var errs []error
		for id := range ids {
			if rexSessionByID(state, id) == nil {
				continue
			}
			if _, err := runRex("kill", id); err != nil {
				errs = append(errs, fmt.Errorf("close imported Rex session %s: %w", id, err))
			}
		}
		return errors.Join(errs...)
	})
}
