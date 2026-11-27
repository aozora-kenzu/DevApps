package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aozora-kenzu/UniDevUI/internal/essentials"
	"github.com/aozora-kenzu/UniDevUI/internal/pkgmgr"
)

func TestExecuteUnifiedSearchTasksProgressiveStreaming(t *testing.T) {
	a := &App{}
	searchCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	releaseSlow := make(chan struct{})
	firstDelivered := make(chan struct{})
	var closeFirst sync.Once

	tasks := []unifiedSearchTask{
		{
			sourceGroup: "winget",
			run: func(ctx context.Context) ([]essentials.WingetTableRow, []pkgmgr.PackageEntry, error) {
				select {
				case <-releaseSlow:
					return []essentials.WingetTableRow{
						{Name: "Obsidian", ID: "Obsidian.Obsidian", Version: "1.8.0", Source: "winget"},
						{Name: "Microsoft Store App", ID: "XP99EXAMPLE", Version: "1.0", Source: "msstore"},
					}, nil, nil
				case <-ctx.Done():
					return nil, nil, ctx.Err()
				}
			},
		},
		{
			sourceGroup: "scoop",
			run: func(ctx context.Context) ([]essentials.WingetTableRow, []pkgmgr.PackageEntry, error) {
				return nil, nil, errors.New("scoop timeout simulation")
			},
		},
		{
			sourceGroup: "js",
			jsMgrs:      []string{"npm", "pnpm"},
			run: func(ctx context.Context) ([]essentials.WingetTableRow, []pkgmgr.PackageEntry, error) {
				return nil, []pkgmgr.PackageEntry{
					{Manager: "npm", ID: "obsidian-cli", Name: "obsidian-cli", Version: "2.0.0"},
				}, nil
			},
		},
	}

	var mu sync.Mutex
	var chunks []UnifiedSearchChunk
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		a.executeUnifiedSearchTasks(searchCtx, cancel, 7, "obsidian", tasks, func(ch UnifiedSearchChunk) {
			mu.Lock()
			chunks = append(chunks, ch)
			mu.Unlock()
			if ch.SourceGroup == "js" {
				closeFirst.Do(func() { close(firstDelivered) })
			}
		})
	}()

	// Fast source ("js") must deliver immediately while "winget" is still blocked waiting on releaseSlow.
	select {
	case <-firstDelivered:
	case <-time.After(2 * time.Second):
		t.Fatal("fast source did not stream before slow source finished")
	}

	mu.Lock()
	var sawWingetYet bool
	for _, ch := range chunks {
		if ch.SourceGroup == "winget" {
			sawWingetYet = true
		}
		if ch.Done {
			t.Fatalf("search marked done before winget finished: %+v", ch)
		}
	}
	mu.Unlock()
	if sawWingetYet {
		t.Fatal("slow winget source should still be pending")
	}

	close(releaseSlow)

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("search did not finish after releasing slow source")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(chunks) != 3 {
		t.Fatalf("expected 3 streamed chunks, got %d (%+v)", len(chunks), chunks)
	}
	final := chunks[len(chunks)-1]
	if !final.Done || final.Completed != 3 || final.Total != 3 || len(final.Pending) != 0 {
		t.Fatalf("unexpected final chunk state: %+v", final)
	}

	seenErr := false
	seenJS := false
	for _, ch := range chunks {
		if ch.SourceGroup == "scoop" && ch.Error != "" {
			seenErr = true
		}
		if ch.SourceGroup == "js" && len(ch.JSMgrs) == 2 && len(ch.PkgRows) == 1 {
			seenJS = true
		}
	}
	if !seenErr || !seenJS {
		t.Fatalf("expected scoop error chunk and js deduplicated chunk, got %+v", chunks)
	}
}

func TestExecuteUnifiedSearchTasksCancellation(t *testing.T) {
	a := &App{}
	searchCtx, cancel := context.WithCancel(context.Background())

	tasks := []unifiedSearchTask{
		{
			sourceGroup: "winget",
			run: func(ctx context.Context) ([]essentials.WingetTableRow, []pkgmgr.PackageEntry, error) {
				<-ctx.Done()
				return nil, nil, ctx.Err()
			},
		},
	}

	var mu sync.Mutex
	emitted := 0
	cancel()
	a.executeUnifiedSearchTasks(searchCtx, cancel, 9, "git", tasks, func(ch UnifiedSearchChunk) {
		mu.Lock()
		emitted++
		mu.Unlock()
	})

	mu.Lock()
	defer mu.Unlock()
	if emitted != 0 {
		t.Fatalf("cancelled search must not emit stale chunks, got %d", emitted)
	}
}
