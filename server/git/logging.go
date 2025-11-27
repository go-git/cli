package git

import (
	"context"
	"io"
	"time"

	"github.com/go-git/go-git/v6/plumbing/protocol/packp"
)

type Logger interface {
	Printf(format string, v ...any)
}

func LoggingMiddleware(logger Logger, next Handler) HandlerFunc {
	return func(ctx context.Context, c io.ReadWriteCloser, r *packp.GitProtoRequest) {
		now := time.Now()

		next.ServeTCP(ctx, c, r)

		elapsedTime := time.Since(now)
		if logger != nil {
			logger.Printf("%s %s %s %v %v", r.Host, r.RequestCommand, r.Pathname, r.ExtraParams, elapsedTime)
		}
	}
}
