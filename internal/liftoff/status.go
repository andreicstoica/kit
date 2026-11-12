package liftoff

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const psqlTimeout = 2 * time.Second

// HasDBs returns, for each name, whether the corresponding database exists.
// A single psql invocation replaces N individual HasDB calls. An empty successful
// result means no matches; query failure is unknown state, never proof of absence.
func HasDBs(names []string) (map[string]bool, error) {
	if len(names) == 0 {
		return nil, nil
	}
	// Build the IN clause safely — names come from our own config, not user input.
	quoted := make([]string, len(names))
	dbToName := make(map[string]string, len(names))
	for i, n := range names {
		db := DBName(n)
		quoted[i] = "'" + db + "'"
		dbToName[db] = n
	}
	query := "SELECT datname FROM pg_database WHERE datname IN (" + strings.Join(quoted, ",") + ")"
	result := make(map[string]bool, len(names))
	for _, n := range names {
		result[n] = false
	}
	ctx, cancel := context.WithTimeout(context.Background(), psqlTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "psql", "-d", "postgres", "-Atc", query)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("check workspace databases: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		db := strings.TrimSpace(line)
		if db == "" {
			continue
		}
		if name, ok := dbToName[db]; ok {
			result[name] = true
		}
	}
	return result, nil
}
