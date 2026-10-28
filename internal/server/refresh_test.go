package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/locales"
)

// TestRefresherSingleFlight: callers that arrive during a build wait for it,
// and skip their own build when it already made the marts fresh.
func TestRefresherSingleFlight(t *testing.T) {
	var builds atomic.Int32
	var fresh atomic.Bool
	release := make(chan struct{})
	r := newRefresher(context.Background(),
		func(context.Context) error {
			builds.Add(1)
			<-release
			fresh.Store(true)
			return nil
		},
		func(context.Context) (bool, error) { return fresh.Load(), nil })

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.run(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	waitFor(t, func() bool { return builds.Load() == 1 && r.pending.Load() == 5 })
	if !r.busy() {
		t.Error("busy() is false during a build")
	}
	close(release)
	wg.Wait()
	if n := builds.Load(); n != 1 {
		t.Errorf("%d builds, want 1", n)
	}
	if r.busy() {
		t.Error("busy() is true after the builds")
	}
}

// TestRefresherError: a failed build is reported until a later one succeeds.
func TestRefresherError(t *testing.T) {
	fail := errors.New("dbt build exited 1")
	var next atomic.Pointer[error]
	next.Store(&fail)
	r := newRefresher(context.Background(),
		func(context.Context) error { return *next.Load() },
		func(context.Context) (bool, error) { return false, nil })
	if err := r.run(context.Background()); !errors.Is(err, fail) || !errors.Is(r.err(), fail) {
		t.Errorf("run = %v, err() = %v", err, r.err())
	}
	var ok error
	next.Store(&ok)
	if err := r.run(context.Background()); err != nil || r.err() != nil {
		t.Errorf("after success: run = %v, err() = %v", err, r.err())
	}
}

// TestServeStopsDuringBuild: cancelling Serve's context while a background
// statistics build runs cancels the build's context (dbt.Build then removes
// its container) and Serve returns only after the build has returned.
func TestServeStopsDuringBuild(t *testing.T) {
	bundle, err := i18n.Load(locales.FS)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s, err := New(ctx, Config{Bundle: bundle})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	var returned atomic.Bool
	s.stats = newRefresher(ctx,
		func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			time.Sleep(50 * time.Millisecond) // the container removal
			returned.Store(true)
			return ctx.Err()
		},
		func(context.Context) (bool, error) { return false, nil })

	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()
	s.stats.trigger()
	<-started
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve: %v", err)
		}
		if !returned.Load() {
			t.Error("Serve returned before the build did")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not stop within 10s")
	}
	if !errors.Is(s.stats.err(), context.Canceled) {
		t.Errorf("build error = %v, want context.Canceled", s.stats.err())
	}
}

// waitFor polls cond until it holds, or fails the test after 5 s.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 5s")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPlainText(t *testing.T) {
	in := "\x1b[0m11:49:46  Compilation Error\n\x1b[31mDone.\x1b[0m PASS=3"
	if got, want := plainText(in), "11:49:46  Compilation Error\nDone. PASS=3"; got != want {
		t.Errorf("plainText = %q, want %q", got, want)
	}
}

// TestRefresherForce: force builds even when the marts are fresh, and a
// second force while one runs starts nothing and returns errBusy.
func TestRefresherForce(t *testing.T) {
	var builds atomic.Int32
	release := make(chan struct{})
	r := newRefresher(context.Background(),
		func(context.Context) error {
			builds.Add(1)
			<-release
			return nil
		},
		func(context.Context) (bool, error) { return true, nil })

	if err := r.force(); err != nil {
		t.Fatalf("first force: %v", err)
	}
	waitFor(t, func() bool { return builds.Load() == 1 })
	if !r.busy() {
		t.Error("busy() is false during a forced build")
	}
	if err := r.force(); !errors.Is(err, errBusy) {
		t.Errorf("second force during a build: %v, want errBusy", err)
	}
	close(release)
	r.wait()
	if n := builds.Load(); n != 1 {
		t.Errorf("%d builds, want 1", n)
	}
	if err := r.force(); err != nil {
		t.Errorf("force after the build: %v", err)
	}
	r.wait()
	if n := builds.Load(); n != 2 {
		t.Errorf("%d builds after the second force, want 2", n)
	}
}
