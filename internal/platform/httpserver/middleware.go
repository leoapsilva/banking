// Package httpserver holds HTTP-layer concerns shared across every
// feature's handler: request logging and request-id propagation.
package httpserver

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	body   *bytes.Buffer // non-nil only when body logging is enabled
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if r.body != nil {
		r.body.Write(b)
	}
	return r.ResponseWriter.Write(b)
}

type contextKey string

const requestIDKey contextKey = "request_id"

const callerKey contextKey = "api_caller"

// RequestID extracts the request id assigned by RequestLogger, if present.
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// WithCaller records the authenticated API caller's name on the context.
func WithCaller(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, callerKey, name)
}

// Caller returns the API caller authenticated by APIKeyAuth, or "" when the
// route was exempt from authentication.
//
// Handlers use this to scope ownership: a caller may only act on resources
// it created, rather than on any id it can guess.
func Caller(ctx context.Context) string {
	if v, ok := ctx.Value(callerKey).(string); ok {
		return v
	}
	return ""
}

// RequestLogger logs every incoming HTTP request — method, path, status,
// duration — at INFO (or WARN/ERROR for 4xx/5xx). When logBody is true
// (LOG_HTTP_BODY=true), it additionally logs the raw request/response
// bodies at DEBUG level. It deliberately never logs headers, so
// Authorization bearer tokens are never written to logs regardless of
// logBody.
func RequestLogger(logBody bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			requestID := uuid.NewString()

			var reqBody []byte
			if logBody && r.Body != nil {
				reqBody, _ = io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(reqBody))
			}

			rec := &statusRecorder{ResponseWriter: w}
			if logBody {
				rec.body = &bytes.Buffer{}
			}

			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			next.ServeHTTP(rec, r.WithContext(ctx))

			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}

			level := slog.LevelInfo
			switch {
			case status >= 500:
				level = slog.LevelError
			case status >= 400:
				level = slog.LevelWarn
			}

			slog.Log(r.Context(), level, "http request",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"query", r.URL.RawQuery,
				"status", status,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)

			if logBody {
				slog.Debug("http request body",
					"request_id", requestID,
					"request_body", string(reqBody),
					"response_body", rec.body.String(),
				)
			}
		})
	}
}
