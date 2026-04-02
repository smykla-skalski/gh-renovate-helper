package tui

import (
	"testing"

	"github.com/klaudiush/gh-renovate-tracker/internal/github"
)

func TestBatchMergeCmd_InvalidRepo(t *testing.T) {
	// Can't call real client, but we can test the command factory returns a func.
	cmd := batchMergeCmd(nil, []github.PR{}, "squash")
	if cmd == nil {
		t.Fatal("batchMergeCmd should return a non-nil cmd")
	}
	// Empty PR list should return success immediately.
	msg := cmd()
	if _, ok := msg.(actionDoneMsg); !ok {
		t.Errorf("empty batch should return actionDoneMsg, got %T", msg)
	}
}

func TestBatchApproveCmd_Empty(t *testing.T) {
	cmd := batchApproveCmd(nil, []github.PR{})
	if cmd == nil {
		t.Fatal("batchApproveCmd should return a non-nil cmd")
	}
	msg := cmd()
	if done, ok := msg.(actionDoneMsg); !ok {
		t.Errorf("empty batch should return actionDoneMsg, got %T", msg)
	} else if done.msg != "Approved 0 PRs" {
		t.Errorf("msg = %q", done.msg)
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
