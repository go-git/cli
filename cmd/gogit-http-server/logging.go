package main

import (
	"log"
	"net/http"
	"strings"
	"time"
)

type logWriter struct {
	http.ResponseWriter
	code, bytes int
}

func (r *logWriter) Write(p []byte) (int, error) {
	written, err := r.ResponseWriter.Write(p)
	r.bytes += written
	return written, err
}

// Note this is generally only called when sending an HTTP error, so it's
// important to set the `code` value to 200 as a default
func (r *logWriter) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

// LoggingMiddleware is the logging middleware where we log incoming and
// outgoing requests for a multiplexer. It should be the first middleware
// called so it can log request times accurately.
func LoggingMiddleware(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addr := r.RemoteAddr
		if colon := strings.LastIndex(addr, ":"); colon != -1 {
			addr = addr[:colon]
		}

		writer := &logWriter{
			ResponseWriter: w,
			code:           http.StatusOK, // default. so important! see above.
		}

		startTime := time.Now()

		next.ServeHTTP(writer, r)

		elapsedTime := time.Since(startTime)
		logger.Printf("%s %s %s %s %d %dB %v", addr, r.Method, r.RequestURI, r.Proto, writer.code, writer.bytes, elapsedTime)
	})
}
