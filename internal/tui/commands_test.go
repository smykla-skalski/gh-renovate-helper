package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/smykla-skalski/gh-renovate-helper/internal/github"
)

func TestBatchMergeCmd_InvalidRepo(t *testing.T) {
	cmd := batchMergeCmd(nil, []github.PR{}, "squash")
	if cmd == nil {
		t.Fatal("batchMergeCmd should return a non-nil cmd")
	}
	msg := cmd()
	// With progress channels, batch cmds return tea.BatchMsg wrapping sub-commands.
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", msg)
	}
	// Execute the batch runner (first cmd) — empty list should succeed immediately.
	var found bool
	for _, c := range batch {
		m := c()
		if _, ok := m.(actionDoneMsg); ok {
			found = true
		}
	}
	if !found {
		t.Error("empty batch should produce actionDoneMsg from one of the sub-commands")
	}
}

func TestBatchApproveCmd_Empty(t *testing.T) {
	cmd := batchApproveCmd(nil, []github.PR{})
	if cmd == nil {
		t.Fatal("batchApproveCmd should return a non-nil cmd")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", msg)
	}
	var found bool
	for _, c := range batch {
		m := c()
		if done, ok := m.(actionDoneMsg); ok {
			found = true
			if done.msg != "Approved 0 PRs" {
				t.Errorf("msg = %q", done.msg)
			}
		}
	}
	if !found {
		t.Error("empty batch should produce actionDoneMsg from one of the sub-commands")
	}
}

func TestAddLabelCmd_InvalidRepo(t *testing.T) {
	pr := github.PR{Repo: "invalid-no-slash", Number: 1}
	cmd := addLabelCmd(nil, pr, "test-label")
	msg := cmd()
	if _, ok := msg.(errMsg); !ok {
		t.Errorf("invalid repo should return errMsg, got %T", msg)
	}
}

func TestRerunChecksCmd_InvalidRepo(t *testing.T) {
	pr := github.PR{Repo: "noslash", Number: 1}
	cmd := rerunChecksCmd(nil, pr)
	msg := cmd()
	if _, ok := msg.(errMsg); !ok {
		t.Errorf("invalid repo should return errMsg, got %T", msg)
	}
}
