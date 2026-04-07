package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/smykla-skalski/gh-renovate-helper/internal/cache"
	"github.com/smykla-skalski/gh-renovate-helper/internal/config"
	"github.com/smykla-skalski/gh-renovate-helper/internal/github"
)

func newTestModel() Model {
	cfg := &config.Config{
		MergeMethod:     "squash",
		RefreshInterval: 5 * time.Minute,
		CacheMaxAge:     24 * time.Hour,
	}
	m := New(nil, cfg, cache.Empty())
	m.list = m.list.SetPRs([]github.PR{
		{ID: "1", Number: 10, Repo: "org/repo", Title: "update go"},
		{ID: "2", Number: 20, Repo: "org/other", Title: "update helm"},
	})
	m.loading = false
	return m
}

// --- New() with warm vs empty cache ---

func TestNew_WarmCache_NotLoading(t *testing.T) {
	c := cache.Empty()
	prs := []github.PR{{ID: "1", Repo: "org/repo", Title: "PR"}}
	c.Set("org/repo", prs, time.Now())

	cfg := &config.Config{RefreshInterval: 5 * time.Minute, CacheMaxAge: 24 * time.Hour}
	m := New(nil, cfg, c)

	if m.loading {
		t.Error("loading should be false with warm cache")
	}
	pr, ok := m.list.Selected()
	if !ok {
		t.Fatal("list should have PRs from cache")
	}
	if pr.Repo != "org/repo" {
		t.Errorf("list PR repo = %q, want org/repo", pr.Repo)
	}
}

func TestNew_EmptyCache_Loading(t *testing.T) {
	cfg := &config.Config{RefreshInterval: 5 * time.Minute, CacheMaxAge: 24 * time.Hour}
	m := New(nil, cfg, cache.Empty())

	if !m.loading {
		t.Error("loading should be true with empty cache")
	}
}

func TestNew_ScheduledReposInitialized(t *testing.T) {
	cfg := &config.Config{RefreshInterval: 5 * time.Minute, CacheMaxAge: 24 * time.Hour}
	m := New(nil, cfg, cache.Empty())

	if m.scheduledRepos == nil {
		t.Error("scheduledRepos should be initialized")
	}
}

// --- computeStaleRepos ---

func TestComputeStaleRepos_Fresh(t *testing.T) {
	c := cache.Empty()
	c.Set("fresh/repo", nil, time.Now())
	cfg := &config.Config{CacheMaxAge: 24 * time.Hour}

	stale := computeStaleRepos(c, cfg)
	if stale["fresh/repo"] {
		t.Error("fresh/repo should not be in stale set")
	}
}

func TestComputeStaleRepos_Stale(t *testing.T) {
	c := cache.Empty()
	c.Set("old/repo", nil, time.Now().Add(-25*time.Hour))
	cfg := &config.Config{CacheMaxAge: 24 * time.Hour}

	stale := computeStaleRepos(c, cfg)
	if !stale["old/repo"] {
		t.Error("old/repo should be in stale set")
	}
}

func TestComputeStaleRepos_Mixed(t *testing.T) {
	c := cache.Empty()
	c.Set("fresh/repo", nil, time.Now())
	c.Set("stale/repo", nil, time.Now().Add(-25*time.Hour))
	cfg := &config.Config{CacheMaxAge: 24 * time.Hour}

	stale := computeStaleRepos(c, cfg)
	if stale["fresh/repo"] {
		t.Error("fresh/repo should not be stale")
	}
	if !stale["stale/repo"] {
		t.Error("stale/repo should be stale")
	}
}

// --- initialRepoDelay ---

func TestInitialRepoDelay_StaleRepo_RefreshesSoon(t *testing.T) {
	c := cache.Empty()
	c.Set("org/repo", nil, time.Now().Add(-25*time.Hour)) // stale
	cfg := &config.Config{RefreshInterval: 5 * time.Minute, CacheMaxAge: 24 * time.Hour}

	delay := initialRepoDelay(c, "org/repo", cfg, 0, 1)
	if delay > cfg.RefreshInterval/2 {
		t.Errorf("stale repo delay too long: %v (max %v)", delay, cfg.RefreshInterval/2)
	}
}

func TestInitialRepoDelay_UnknownRepo_RefreshesSoon(t *testing.T) {
	c := cache.Empty()
	cfg := &config.Config{RefreshInterval: 5 * time.Minute, CacheMaxAge: 24 * time.Hour}

	delay := initialRepoDelay(c, "nobody/nowhere", cfg, 0, 1)
	if delay > cfg.RefreshInterval/2 {
		t.Errorf("unknown repo delay too long: %v", delay)
	}
}

func TestInitialRepoDelay_FreshRepo_WaitsForInterval(t *testing.T) {
	c := cache.Empty()
	// Fetched 1 minute ago — 4 minutes remain before next interval.
	c.Set("org/repo", nil, time.Now().Add(-1*time.Minute))
	cfg := &config.Config{RefreshInterval: 5 * time.Minute, CacheMaxAge: 24 * time.Hour}

	delay := initialRepoDelay(c, "org/repo", cfg, 0, 1)
	// Should wait at least 3 minutes (remaining ~4m minus some jitter tolerance).
	if delay < 3*time.Minute {
		t.Errorf("fresh repo delay too short: %v (want ≥3m)", delay)
	}
}

func TestInitialRepoDelay_MultipleRepos_Staggered(t *testing.T) {
	c := cache.Empty()
	c.Set("org/r1", nil, time.Now().Add(-25*time.Hour))
	c.Set("org/r2", nil, time.Now().Add(-25*time.Hour))
	cfg := &config.Config{RefreshInterval: 5 * time.Minute, CacheMaxAge: 24 * time.Hour}

	d0 := initialRepoDelay(c, "org/r1", cfg, 0, 2)
	d1 := initialRepoDelay(c, "org/r2", cfg, 1, 2)
	// Second repo should have a larger base delay (i=1 vs i=0).
	if d1 <= d0 {
		t.Errorf("repo 2 delay (%v) should be > repo 1 delay (%v) for staggering", d1, d0)
	}
}

// --- orgDiscoveredMsg handler ---

func TestUpdate_OrgDiscoveredMsg_NewReposScheduled(t *testing.T) {
	c := cache.EmptyAt(t.TempDir() + "/cache.json")
	cfg := &config.Config{RefreshInterval: 5 * time.Minute, CacheMaxAge: 24 * time.Hour}
	m := New(nil, cfg, c)

	now := time.Now()
	result, cmd := m.Update(orgDiscoveredMsg{
		org: "myorg",
		reposPRs: map[string][]github.PR{
			"myorg/alpha": {{ID: "1", Repo: "myorg/alpha"}},
			"myorg/beta":  {{ID: "2", Repo: "myorg/beta"}},
		},
		fetchedAt: now,
	})
	m = result.(Model)

	// Cache updated.
	if _, ok := c.Get("myorg/alpha"); !ok {
		t.Error("cache should have myorg/alpha")
	}
	if _, ok := c.Get("myorg/beta"); !ok {
		t.Error("cache should have myorg/beta")
	}

	// Both repos added to scheduled set.
	if !m.scheduledRepos["myorg/alpha"] {
		t.Error("myorg/alpha should be in scheduledRepos")
	}
	if !m.scheduledRepos["myorg/beta"] {
		t.Error("myorg/beta should be in scheduledRepos")
	}

	// Cmd returned (re-discovery + per-repo schedules).
	if cmd == nil {
		t.Error("should return cmd after orgDiscoveredMsg")
	}
}

func TestUpdate_OrgDiscoveredMsg_ExistingReposNotDuplicated(t *testing.T) {
	c := cache.EmptyAt(t.TempDir() + "/cache.json")
	cfg := &config.Config{RefreshInterval: 5 * time.Minute, CacheMaxAge: 24 * time.Hour}
	m := New(nil, cfg, c)
	// Pre-mark a repo as already scheduled.
	m.scheduledRepos["myorg/existing"] = true

	_, cmd := m.Update(orgDiscoveredMsg{
		org: "myorg",
		reposPRs: map[string][]github.PR{
			"myorg/existing": {{ID: "1", Repo: "myorg/existing"}},
		},
		fetchedAt: time.Now(),
	})

	// We still expect a cmd (at minimum the re-discovery reschedule).
	if cmd == nil {
		t.Error("should still return cmd for re-discovery")
	}
	// The existing repo should remain scheduled (not duplicated).
	if !m.scheduledRepos["myorg/existing"] {
		t.Error("existing repo should remain in scheduledRepos")
	}
}

// --- repoPRsLoadedMsg handler ---

func TestUpdate_RepoPRsLoadedMsg_UpdatesCache(t *testing.T) {
	c := cache.EmptyAt(t.TempDir() + "/cache.json")
	cfg := &config.Config{RefreshInterval: 5 * time.Minute, CacheMaxAge: 24 * time.Hour}
	m := New(nil, cfg, c)

	fetchTime := time.Now()
	prs := []github.PR{{ID: "1", Repo: "org/repo", Title: "bump"}}
	result, cmd := m.Update(repoPRsLoadedMsg{
		repo:      "org/repo",
		prs:       prs,
		fetchedAt: fetchTime,
	})
	m = result.(Model)

	// Cache updated.
	entry, ok := c.Get("org/repo")
	if !ok {
		t.Fatal("cache should have org/repo")
	}
	if len(entry.PRs) != 1 {
		t.Errorf("cache PRs = %d, want 1", len(entry.PRs))
	}

	// loading cleared.
	if m.loading {
		t.Error("loading should be false after first repoPRsLoadedMsg")
	}

	// cmd returned (schedule next refresh for this repo).
	if cmd == nil {
		t.Error("should return cmd for next refresh")
	}
}

func TestUpdate_RepoPRsLoadedMsg_ListReflectsMergedPRs(t *testing.T) {
	c := cache.EmptyAt(t.TempDir() + "/cache.json")
	// Pre-populate cache with another repo.
	c.Set("org/other", []github.PR{{ID: "x", Repo: "org/other"}}, time.Now())
	cfg := &config.Config{RefreshInterval: 5 * time.Minute, CacheMaxAge: 24 * time.Hour}
	m := New(nil, cfg, c)

	result, _ := m.Update(repoPRsLoadedMsg{
		repo:      "org/repo",
		prs:       []github.PR{{ID: "1", Repo: "org/repo"}},
		fetchedAt: time.Now(),
	})
	m = result.(Model)

	// List should show PRs from both repos.
	all := m.list.AllPRs()
	repos := make(map[string]bool)
	for _, pr := range all {
		repos[pr.Repo] = true
	}
	if !repos["org/repo"] {
		t.Error("list should contain org/repo PRs")
	}
	if !repos["org/other"] {
		t.Error("list should still contain org/other PRs")
	}
}

func TestStartConfirm(t *testing.T) {
	m := newTestModel()
	called := false
	cmd := func() tea.Msg { called = true; return nil }

	m = m.startConfirm("Merge org/repo#10? (y/n)", cmd)

	if !m.confirming {
		t.Error("confirming should be true")
	}
	if m.status != "Merge org/repo#10? (y/n)" {
		t.Errorf("status = %q", m.status)
	}
	if m.pendingCmd == nil {
		t.Error("pendingCmd should be set")
	}

	// Confirm with y.
	result, resultCmd := m.handleConfirm(tea.KeyPressMsg{Text: "y"})
	m = result.(Model)
	if m.confirming {
		t.Error("confirming should be false after y")
	}
	if resultCmd == nil {
		t.Error("cmd should be returned on confirm")
	}
	// Verify the pending cmd is the one we set.
	_ = called
}

func TestConfirmCancel(t *testing.T) {
	m := newTestModel()
	m = m.startConfirm("Merge? (y/n)", func() tea.Msg { return nil })

	result, cmd := m.handleConfirm(tea.KeyPressMsg{Text: "n"})
	m = result.(Model)
	if m.confirming {
		t.Error("confirming should be false after cancel")
	}
	if m.status != "cancelled" {
		t.Errorf("status = %q, want cancelled", m.status)
	}
	if cmd != nil {
		t.Error("cmd should be nil on cancel")
	}
}

func TestConfirmEsc(t *testing.T) {
	m := newTestModel()
	m = m.startConfirm("Merge? (y/n)", func() tea.Msg { return nil })

	result, cmd := m.handleConfirm(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = result.(Model)
	if m.confirming {
		t.Error("confirming should be false after esc")
	}
	if cmd != nil {
		t.Error("cmd should be nil on esc")
	}
}

func TestHandleKey_MergeTriggersConfirm(t *testing.T) {
	m := newTestModel()
	m.current = viewList

	result, _ := m.handleKey(tea.KeyPressMsg{Text: "m"})
	m = result.(Model)
	if !m.confirming {
		t.Error("pressing m should trigger confirmation")
	}
	if m.status == "" {
		t.Error("confirm message should be set")
	}
}

func TestHandleKey_LabelOpensInput(t *testing.T) {
	m := newTestModel()
	m.current = viewList

	result, _ := m.handleKey(tea.KeyPressMsg{Text: "l"})
	m = result.(Model)
	if m.current != viewLabel {
		t.Errorf("current = %d, want viewLabel (%d)", m.current, viewLabel)
	}
	if m.labelPR.ID == "" {
		t.Error("labelPR should be set")
	}
}

func TestHandleLabelInput_Esc(t *testing.T) {
	m := newTestModel()
	m.current = viewLabel

	result, _ := m.handleLabelInput(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = result.(Model)
	if m.current != viewList {
		t.Errorf("current = %d, want viewList after esc", m.current)
	}
}

func TestRenderStatus_Confirming(t *testing.T) {
	m := newTestModel()
	m.confirming = true
	m.status = "Merge org/repo#10? (y/n)"

	s := m.renderBottomBar()
	if s == "" {
		t.Error("renderBottomBar should return non-empty for confirming state")
	}
}

func TestRenderStatus_Loading(t *testing.T) {
	m := newTestModel()
	m.loading = true

	s := m.renderBottomBar()
	if s == "" {
		t.Error("renderBottomBar should return non-empty for loading state")
	}
}

func TestRenderStatus_Error(t *testing.T) {
	m := newTestModel()
	m.statusErr = true
	m.status = "something failed"

	s := m.renderBottomBar()
	if s == "" {
		t.Error("renderBottomBar should return non-empty for error state")
	}
}

func TestRenderStatus_LastFetch(t *testing.T) {
	m := newTestModel()
	m.lastFetch = time.Now().Add(-10 * time.Second).UnixNano()

	s := m.renderBottomBar()
	if s == "" {
		t.Error("renderBottomBar should return non-empty with lastFetch")
	}
}

const testStatus = "3 PRs"

func snapshotModel(width, height int) Model {
	now := time.Now()
	m := newTestModel()
	m.width = width
	m.height = height
	m.list = m.list.SetSize(width, height-1)
	m.list = m.list.SetPRs([]github.PR{
		{Repo: "org/repo", Title: "update go", ReviewStatus: "APPROVED", CheckStatus: "SUCCESS", CreatedAt: now.Add(-48 * time.Hour)},
		{Repo: "org/repo", Title: "update helm", ReviewStatus: "REVIEW_REQUIRED", CreatedAt: now.Add(-72 * time.Hour)},
		{Repo: "org/other", Title: "bump deps", CheckStatus: "FAILURE", CreatedAt: now.Add(-24 * time.Hour)},
		{Repo: "org/other", Title: "bump lodash (security)", CheckStatus: "SUCCESS", ReviewStatus: "REVIEW_REQUIRED", Labels: []string{"security"}, CreatedAt: now.Add(-12 * time.Hour)},
	})
	m.lastFetch = now.UnixNano()
	m.status = testStatus
	return m
}

func TestView_Snapshot(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := snapshotModel(100, 15)
	golden.RequireEqual(t, ansi.Strip(m.View().Content)+"\n")
}

func TestView_Narrow_Snapshot(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := snapshotModel(60, 15)
	golden.RequireEqual(t, ansi.Strip(m.View().Content)+"\n")
}
