package leader

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestLocalElectorAcquire(t *testing.T) {
	l := slog.New(slog.NewTextHandler(os.Stderr, nil))
	elector := New(nil, nil, l)
	if err := elector.Acquire(context.Background()); err != nil {
		t.Fatalf("acquire leader: %v", err)
	}
}
