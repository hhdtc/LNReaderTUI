package site

import (
	"context"
	"io"
	"time"
)

func ioReadAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }

func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}
