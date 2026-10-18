package search

import (
	"context"
	"testing"
)

// Regression test for the old nil dereference at search.go:39: Telegram can
// deliver an update without a Message (edited messages, callback-only updates),
// and the default handler used to read update.Message.Text unconditionally.
func TestNewPaginatorWithNilUpdate(t *testing.T) {
	p := NewPaginator(context.Background(), nil, nil)
	if p.query != "" {
		t.Errorf("query = %q, want empty", p.query)
	}
}
