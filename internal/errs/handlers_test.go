package errs

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "database timeout",
			err:  fmt.Errorf("%w: query timed out", ErrDatabaseTimeout),
			want: http.StatusGatewayTimeout,
		},
		{
			name: "LLM timeout",
			err:  fmt.Errorf("%w: provider timed out", ErrLLMTimeout),
			want: http.StatusGatewayTimeout,
		},
		{
			name: "generic deadline exceeded",
			err:  context.DeadlineExceeded,
			want: http.StatusRequestTimeout,
		},
		{
			name: "nil error",
			err:  nil,
			want: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HTTPStatus(tt.err); got != tt.want {
				t.Errorf("HTTPStatus(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
