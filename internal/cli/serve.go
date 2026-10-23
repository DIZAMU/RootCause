package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/DIZAMU/RootCause/internal/server"
	"github.com/DIZAMU/RootCause/internal/session"
	"github.com/DIZAMU/RootCause/internal/store"
	"github.com/DIZAMU/RootCause/web"
)

// reapInterval is how often the long-running dashboard server reaps expired
// sessions (spec section 8.4 step 8).
const reapInterval = time.Minute

// Serve runs the local dashboard on 127.0.0.1 (spec sections 10 and 11) until
// Ctrl-C. It reaps expired sessions on a ticker while it runs.
func (a *App) Serve(ctx context.Context, args []string) error {
	port, ok := parsePort(args)
	if !ok {
		a.say("serve.usage", nil)
		return nil
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv, err := server.New(ctx, server.Config{
		Store:        a.st,
		Engine:       a.eng,
		Bundle:       a.bundle,
		LearnerID:    a.learner.ID,
		ExercisesDir: a.exercisesDir(),
		Catalog:      func() ([]store.ExerciseRow, error) { return loadAllExercises(a.root) },
		BuildStats:   a.buildStats,
		Describe:     func(lang string, err error) string { return Message(a.bundle, lang, err) },
		Assets:       web.Assets(),
	})
	if err != nil {
		return err
	}
	ln, err := server.Listen(port)
	if err != nil {
		return failWith("serve.listen_failed", map[string]string{"port": strconv.Itoa(port)}, err)
	}
	if web.Assets() == nil {
		a.say("serve.not_built", nil)
	}
	a.say("serve.listening", map[string]string{"url": fmt.Sprintf("http://localhost:%d", port)})

	var reaper sync.WaitGroup
	reaper.Add(1)
	go func() {
		defer reaper.Done()
		session.NewReaper(a.eng, a.st, reapInterval).Run(ctx)
	}()
	err = srv.Serve(ctx, ln)
	stop()
	reaper.Wait()
	a.say("serve.stopped", nil)
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// parsePort reads "[--port N]".
func parsePort(args []string) (int, bool) {
	switch {
	case len(args) == 0:
		return server.DefaultPort, true
	case len(args) == 2 && args[0] == "--port":
		p, err := strconv.Atoi(args[1])
		if err != nil || p < 1 || p > 65535 {
			return 0, false
		}
		return p, true
	}
	return 0, false
}
