package liftoff

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// PRStatus is a fresh GitHub lookup. NONE means the lookup succeeded without
// a matching PR; Err means the remote state is unknown.
type PRStatus struct {
	Number  int
	URL     string
	State   string
	HeadOID string
	Draft   bool
	Err     error
}

// RemotePRStatuses queries only the requested branches, including all pages
// of their PR history. An open PR takes precedence over terminal PRs. Otherwise
// the newest PR wins, so an old closure cannot mark reused branches for wash.
func (l Layout) RemotePRStatuses(branches []string) (map[string]PRStatus, error) {
	result := make(map[string]PRStatus)
	var wanted []string
	for _, branch := range branches {
		if branch != "" {
			if _, ok := result[branch]; !ok {
				wanted = append(wanted, branch)
				result[branch] = PRStatus{State: "UNKNOWN"}
			}
		}
	}
	if len(wanted) == 0 {
		return result, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := ghOutput(ctx, l.Master, "repo", "view", "--json", "nameWithOwner")
	if err != nil {
		return result, err
	}
	var repo struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := json.Unmarshal(out, &repo); err != nil {
		return result, fmt.Errorf("decode GitHub repository: %w", err)
	}
	owner, name, ok := strings.Cut(repo.NameWithOwner, "/")
	if !ok || owner == "" || name == "" {
		return result, fmt.Errorf("invalid GitHub repository %q", repo.NameWithOwner)
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var errs []error
	workers := make(chan struct{}, 4)
	for _, branch := range wanted {
		workers <- struct{}{}
		wg.Add(1)
		go func(branch string) {
			defer wg.Done()
			defer func() { <-workers }()
			status, err := remotePRStatus(ctx, l.Master, repo.NameWithOwner, owner, branch)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				status = PRStatus{State: "UNKNOWN", Err: err}
				errs = append(errs, fmt.Errorf("%s: %w", branch, err))
			}
			result[branch] = status
		}(branch)
	}
	wg.Wait()
	return result, errors.Join(errs...)
}

func ghOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("GitHub PR lookup: %w", ctx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("GitHub PR lookup: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func remotePRStatus(ctx context.Context, dir, repo, owner, branch string) (PRStatus, error) {
	out, err := ghOutput(ctx, dir, "api", "repos/"+repo+"/pulls", "--method", "GET",
		"--paginate", "--slurp", "-f", "state=all", "-f", "head="+owner+":"+branch,
		"-f", "sort=created", "-f", "direction=desc", "-f", "per_page=100")
	if err != nil {
		return PRStatus{}, err
	}
	var pages [][]struct {
		Number   int    `json:"number"`
		URL      string `json:"html_url"`
		State    string `json:"state"`
		Draft    bool   `json:"draft"`
		MergedAt string `json:"merged_at"`
		Head     struct {
			Ref  string `json:"ref"`
			SHA  string `json:"sha"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
	}
	if err := json.Unmarshal(out, &pages); err != nil {
		return PRStatus{}, fmt.Errorf("decode GitHub PRs: %w", err)
	}
	status := PRStatus{State: "NONE"}
	for _, page := range pages {
		for _, pr := range page {
			if pr.Head.Ref != branch || !strings.EqualFold(pr.Head.Repo.FullName, repo) {
				continue
			}
			state := strings.ToUpper(pr.State)
			if state != "OPEN" && state != "CLOSED" {
				return PRStatus{}, fmt.Errorf("invalid state %q for PR #%d", pr.State, pr.Number)
			}
			if pr.MergedAt != "" {
				state = "MERGED"
			}
			if status.State == "OPEN" && state != "OPEN" {
				continue
			}
			if (state == "OPEN" && status.State != "OPEN") || pr.Number > status.Number {
				status = PRStatus{Number: pr.Number, URL: pr.URL, State: state, HeadOID: pr.Head.SHA, Draft: pr.Draft}
			}
		}
	}
	return status, nil
}
