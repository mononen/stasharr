package worker

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/mononen/stasharr/internal/models"
)

// WorkerStatus reports the runtime state of a single worker pool.
type WorkerStatus struct {
	Name     string `json:"name"`
	Running  bool   `json:"running"`
	PoolSize int    `json:"pool_size"`
}

// SupervisorStatus is a snapshot of all worker pool states.
type SupervisorStatus struct {
	Workers []WorkerStatus `json:"workers"`
}

// Supervisor manages all worker goroutines.
type Supervisor struct {
	app      *models.App
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.RWMutex
	statuses []WorkerStatus
}

// NewSupervisor creates a new worker supervisor.
func NewSupervisor(app *models.App) *Supervisor {
	return &Supervisor{
		app: app,
	}
}

// poolSize reads an integer pool size from config, returning def on any error
// or when the value is absent.
func (s *Supervisor) poolSize(key string, def int) int {
	raw := s.app.Config.Get(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return def
	}
	return n
}

// setStatus records a WorkerStatus entry under the mutex.
func (s *Supervisor) setStatus(name string, poolSize int, running bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, ws := range s.statuses {
		if ws.Name == name {
			s.statuses[i].Running = running
			s.statuses[i].PoolSize = poolSize
			return
		}
	}
	s.statuses = append(s.statuses, WorkerStatus{Name: name, Running: running, PoolSize: poolSize})
}

// startPool launches n goroutines that each create a fresh worker instance
// and restart it with exponential backoff whenever it exits unexpectedly.
func (s *Supervisor) startPool(ctx context.Context, name string, n int, newWorker func() Worker) {
	s.setStatus(name, n, true)

	for i := 0; i < n; i++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()

			backoff := time.Second
			const maxBackoff = 30 * time.Second

			for {
				w := newWorker()

				func() {
					defer func() {
						if r := recover(); r != nil {
							// panic recovered; fall through to backoff / restart
						}
					}()
					w.Start(ctx)
				}()

				// If the context is done, stop the restart loop.
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Exponential backoff before restarting.
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}

				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
		}()
	}
}

// Start launches all worker pools. It is non-blocking; workers run in
// background goroutines tracked by s.wg.
func (s *Supervisor) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	logger := zerolog.Nop()
	app := s.app

	resolverSize := s.poolSize("pipeline.resolver_pool_size", 1)
	searchSize := s.poolSize("pipeline.search_pool_size", 2)
	downloadSize := s.poolSize("pipeline.download_pool_size", 2)
	moveSize := s.poolSize("pipeline.move_pool_size", 2)
	scanSize := s.poolSize("pipeline.scan_pool_size", 2)

	s.startPool(ctx, "resolver", resolverSize, func() Worker {
		return NewResolverWorker(app, logger)
	})
	s.startPool(ctx, "search", searchSize, func() Worker {
		return NewSearchWorker(app, logger)
	})
	s.startPool(ctx, "download", downloadSize, func() Worker {
		return NewDownloadWorker(app, logger)
	})
	s.startPool(ctx, "move", moveSize, func() Worker {
		return NewMoveWorker(app, logger)
	})
	s.startPool(ctx, "scan", scanSize, func() Worker {
		return NewScanWorker(app, logger)
	})
	s.startPool(ctx, "monitor", 1, func() Worker {
		return NewMonitorWorker(app, logger)
	})
}

// Stop cancels the supervisor context and waits up to 30 seconds for all
// workers to exit.
func (s *Supervisor) Stop() {
	if s.cancel != nil {
		s.cancel()
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
}

// Status returns a snapshot of all worker pool states.
func (s *Supervisor) Status() SupervisorStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := make([]WorkerStatus, len(s.statuses))
	copy(snapshot, s.statuses)
	return SupervisorStatus{Workers: snapshot}
}
