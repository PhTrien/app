package httpapi

import (
	"context"
	"net/http"
	"time"
)

// contextWithTimeout tạo context có thời hạn từ request.
func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}
