package security

import (
	"context"
	"sync"
)

type requestStateKey struct{}

// RequestState stores canonical request authentication data.
type RequestState struct {
	User    string
	Roles   []string
	Claims  map[string]any
	TraceID string
	Token   string
}

var requestStatePool = sync.Pool{New: func() any { return &RequestState{} }}

// RequestStateFromContext returns canonical request state when enabled.
func RequestStateFromContext(ctx context.Context) (*RequestState, bool) {
	if ctx == nil {
		return nil, false
	}
	state, ok := ctx.Value(requestStateKey{}).(*RequestState)
	return state, ok && state != nil
}

// AcquireRequestState obtains a clean state from the framework pool.
func AcquireRequestState() *RequestState { return requestStatePool.Get().(*RequestState) }

// ReleaseRequestState sanitizes and returns state to the framework pool.
func ReleaseRequestState(state *RequestState) {
	if state == nil {
		return
	}
	state.Reset()
	requestStatePool.Put(state)
}

// WithRequestState attaches canonical state to context.
func WithRequestState(ctx context.Context, state *RequestState) context.Context {
	return context.WithValue(ctx, requestStateKey{}, state)
}

// Reset clears sensitive data before state reuse.
func (state *RequestState) Reset() {
	state.User = ""
	state.Claims = nil
	state.Roles = nil
	state.TraceID = ""
	state.Token = ""
}
