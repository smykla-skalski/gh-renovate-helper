package tui

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/klaudiush/gh-renovate-tracker/internal/config"
	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

type (
	prsLoadedMsg     struct{ prs []github.PR }
	errMsg           struct{ err error }
	actionDoneMsg    struct{ msg string }
	batchProgressMsg struct {
		ch    <-chan tea.Msg
		done  int
		total int
		verb  string
		cur   string // e.g. "owner/repo#123"
	}
)

func fetchPRsCmd(client *github.Client, cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		prs, err := client.FetchPRs(cfg)
		if err != nil {
			return errMsg{err}
		}
		return prsLoadedMsg{prs}
	}
}

func mergePRCmd(client *github.Client, pr github.PR, method string) tea.Cmd {
	return func() tea.Msg {
		if err := client.MergePR(pr.ID, method); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{msg: "Merged " + pr.Repo + "#" + strconv.Itoa(pr.Number)}
	}
}

func approvePRCmd(client *github.Client, pr github.PR) tea.Cmd {
	return func() tea.Msg {
		if err := client.ApprovePR(pr.ID); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{msg: "Approved " + pr.Repo + "#" + strconv.Itoa(pr.Number)}
	}
}

func addLabelCmd(client *github.Client, pr github.PR, label string) tea.Cmd {
	return func() tea.Msg {
		parts := strings.SplitN(pr.Repo, "/", 2)
		if len(parts) != 2 {
			return errMsg{err: fmt.Errorf("invalid repo: %s", pr.Repo)}
		}
		if err := client.AddLabel(parts[0], parts[1], pr.Number, label); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{msg: fmt.Sprintf("Added label %q to %s#%d", label, pr.Repo, pr.Number)}
	}
}

func runBatch(prs []github.PR, verb string, fn func(github.PR) error, progressCh chan tea.Msg) tea.Msg {
	slog.Info("batch start", "verb", verb, "count", len(prs))
	errs := make([]error, len(prs))
	var done atomic.Int32
	total := len(prs)
	var wg sync.WaitGroup
	for i := range prs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = fn(prs[i])
			n := int(done.Add(1))
			progressCh <- batchProgressMsg{
				ch:    progressCh,
				done:  n,
				total: total,
				verb:  verb,
				cur:   fmt.Sprintf("%s#%d", prs[i].Repo, prs[i].Number),
			}
		}(i)
	}
	wg.Wait()
	var count int
	for i, err := range errs {
		if err != nil {
			return errMsg{err: fmt.Errorf("%s %s#%d: %w", verb, prs[i].Repo, prs[i].Number, err)}
		}
		count++
	}
	past := strings.ToUpper(verb[:1]) + verb[1:] + "d"
	slog.Info("batch complete", "verb", verb, "count", count)
	return actionDoneMsg{msg: fmt.Sprintf("%s %d PRs", past, count)}
}

func listenProgress(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func batchMergeCmd(client *github.Client, prs []github.PR, method string) tea.Cmd {
	ch := make(chan tea.Msg, len(prs))
	return tea.Batch(
		func() tea.Msg {
			return runBatch(prs, "merge", func(pr github.PR) error {
				return client.MergePR(pr.ID, method)
			}, ch)
		},
		listenProgress(ch),
	)
}

func batchApproveCmd(client *github.Client, prs []github.PR) tea.Cmd {
	ch := make(chan tea.Msg, len(prs))
	return tea.Batch(
		func() tea.Msg {
			return runBatch(prs, "approve", func(pr github.PR) error {
				return client.ApprovePR(pr.ID)
			}, ch)
		},
		listenProgress(ch),
	)
}

func rerunChecksCmd(client *github.Client, pr github.PR) tea.Cmd {
	return func() tea.Msg {
		parts := strings.SplitN(pr.Repo, "/", 2)
		if len(parts) != 2 {
			return errMsg{err: fmt.Errorf("invalid repo: %s", pr.Repo)}
		}
		var suiteIDs []string
		for _, cr := range pr.Checks {
			if cr.Conclusion == "FAILURE" || cr.Conclusion == "TIMED_OUT" {
				if cr.SuiteID != "" {
					suiteIDs = append(suiteIDs, cr.SuiteID)
				}
			}
		}
		if err := client.RerunChecks(parts[0], parts[1], suiteIDs); err != nil {
			return errMsg{err}
		}
		return actionDoneMsg{msg: "Rerun checks for " + pr.Repo + "#" + strconv.Itoa(pr.Number)}
	}
}
