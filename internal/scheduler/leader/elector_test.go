package leader

import (
	"context"
	"testing"

	"github.com/example/go-ai-scheduler/internal/pkg/logger"
)

func TestLocalElectorAcquire(t *testing.T) {
	elector := New(nil, nil, logger.New("test-leader"))
	if err := elector.Acquire(context.Background()); err != nil {
		t.Fatalf("acquire leader: %v", err)
	}
}
