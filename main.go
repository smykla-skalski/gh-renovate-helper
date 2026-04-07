package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/smykla-skalski/gh-renovate-helper/internal/cache"
	"github.com/smykla-skalski/gh-renovate-helper/internal/config"
	"github.com/smykla-skalski/gh-renovate-helper/internal/github"
	"github.com/smykla-skalski/gh-renovate-helper/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		orgs            = flag.String("orgs", "", "comma-separated orgs to track")
		repos           = flag.String("repos", "", "comma-separated owner/repo pairs")
		author          = flag.String("author", "", "PR author (default: renovate[bot])")
		mergeMethod     = flag.String("merge-method", "", "merge|squash|rebase")
		refreshInterval = flag.Duration("refresh", 0, "refresh interval (e.g. 5m)")
		cacheMaxAge     = flag.Duration("cache-max-age", 0, "max cache age before showing stale indicator (e.g. 24h)")
		printOnly       = flag.Bool("print", false, "print PRs to stdout and exit")
		clearCacheFlag  = flag.Bool("clear-cache", false, "delete the local PR cache and exit")
	)
	flag.Parse()

	if *clearCacheFlag {
		if err := cache.Empty().Clear(); err != nil {
			return fmt.Errorf("clear cache: %w", err)
		}
		fmt.Println("cache cleared")
		return nil
	}

	logFile, err := os.OpenFile("/tmp/renovate-helper.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("log file: %w", err)
	}
	defer func() { _ = logFile.Close() }()
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug})))

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if *orgs != "" {
		cfg.Orgs = strings.Split(*orgs, ",")
	}
	if *repos != "" {
		cfg.Repos = strings.Split(*repos, ",")
	}
	if *author != "" {
		cfg.Author = *author
	}
	if *mergeMethod != "" {
		cfg.MergeMethod = *mergeMethod
	}
	if *refreshInterval != 0 {
		cfg.RefreshInterval = *refreshInterval
	}
	if *cacheMaxAge != 0 {
		cfg.CacheMaxAge = *cacheMaxAge
	}

	client, err := github.NewClient()
	if err != nil {
		return fmt.Errorf("github client: %w", err)
	}

	if *printOnly {
		var prs []github.PR
		prs, err = client.FetchPRs(cfg)
		if err != nil {
			return fmt.Errorf("fetch: %w", err)
		}
		for i := range prs {
			fmt.Printf("%s #%d %s [%s] [%s]\n",
				prs[i].Repo, prs[i].Number, prs[i].Title, prs[i].ReviewStatus, prs[i].CheckStatus)
		}
		return nil
	}

	c, err := cache.Load()
	if err != nil {
		slog.Warn("failed to load cache, starting fresh", "error", err)
		c = cache.Empty()
	}

	p := tea.NewProgram(tui.New(client, cfg, c))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
