package ping

import (
	"context"
	"errors"
	"testing"

	"github.com/ManiaSacha/dev-mcp/internal/version"
)

func TestCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler Handler
		want    Result
	}{
		{
			name:    "defaults",
			handler: Handler{},
			want: Result{
				Status:  "ok",
				Version: version.Version,
				Name:    version.Name,
			},
		},
		{
			name:    "explicit identity",
			handler: Handler{Name: "custom", Version: "9.9.9"},
			want: Result{
				Status:  "ok",
				Version: "9.9.9",
				Name:    "custom",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, got, err := tt.handler.Call(t.Context(), nil, Args{})
			if err != nil {
				t.Fatalf("Call() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Call() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCallCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, _, err := Handler{Name: version.Name, Version: version.Version}.Call(ctx, nil, Args{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Call() error = %v, want context.Canceled", err)
	}
}
