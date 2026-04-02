package tui

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/klaudiush/gh-renovate-tracker/internal/config"
	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

type (
	prsLoadedMsg  struct{ prs []github.PR }
	errMsg        struct{ err error }
	actionDoneMsg struct{ msg string }
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

func runBatch(prs []github.PR, verb string, fn func(github.PR) error) tea.Msg {
	errs := make([]error, len(prs))
	var wg sync.WaitGroup
	for i := range prs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = fn(prs[i])
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
	return actionDoneMsg{msg: fmt.Sprintf("%s %d PRs", past, count)}
}

func batchMergeCmd(client *github.Client, prs []github.PR, method string) tea.Cmd {
	return func() tea.Msg {
		return runBatch(prs, "merge", func(pr github.PR) error {
			return client.MergePR(pr.ID, method)
		})
	}
}

func batchApproveCmd(client *github.Client, prs []github.PR) tea.Cmd {
	return func() tea.Msg {
		return runBatch(prs, "approve", func(pr github.PR) error {
			return client.ApprovePR(pr.ID)
		})
	}
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
