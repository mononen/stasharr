package worker

import "context"

// Supervisor manages all worker goroutines.
type Supervisor struct {
	// TODO: add fields
}

// NewSupervisor creates a new worker supervisor.
func NewSupervisor(app interface{}) *Supervisor {
	return &Supervisor{}
}

// Start begins all worker pools.
func (s *Supervisor) Start(ctx context.Context) {
	// TODO: implement
}

// Stop gracefully shuts down all workers.
func (s *Supervisor) Stop() {
	// TODO: implement
}
