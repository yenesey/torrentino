// Package core contains the domain layer shared by the Telegram bot and the
// web interface: list items, available actions, service calls and screen
// descriptors. It knows nothing about Telegram, HTML or HTTP.
package core

import (
	"errors"
	"fmt"
)

// Action is one operation available for a list item. ID is the protocol value
// (the same string Telegram puts into callback data) and Label is what both
// interfaces show.
type Action struct {
	ID    string
	Label string
}

var (
	// ErrStaleItem means the selected item is no longer in the server-side
	// list (it was deleted or the list was reloaded).
	ErrStaleItem = errors.New("item is no longer in the list")
	// ErrNotAvailable means the requested action is not offered for the item.
	ErrNotAvailable = errors.New("action is not available")
	// ErrUpstream means Jackett, Transmission or Torrserver failed.
	ErrUpstream = errors.New("upstream service failed")
	// ErrUnsafePath means a delete target is outside the configured roots.
	ErrUnsafePath = errors.New("path is outside the configured download roots")
	// ErrTorrentTooLarge means a .torrent download exceeded the size limit.
	ErrTorrentTooLarge = errors.New("torrent file is too large")
	// ErrInvalidInput means a query or command value is syntactically invalid.
	ErrInvalidInput = errors.New("invalid input")
)

// Upstream wraps an error while keeping ErrUpstream in the chain.
func Upstream(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrUpstream, err)
}

// ActionIDs maps actions to their protocol IDs. The Telegram keyboard uses
// those IDs as button labels and callback payloads.
func ActionIDs(actions []Action) []string {
	result := make([]string, 0, len(actions))
	for _, action := range actions {
		result = append(result, action.ID)
	}
	return result
}

// ActionsByID indexes actions by their protocol ID.
func ActionsByID(actions []Action) map[string]Action {
	result := make(map[string]Action, len(actions))
	for _, action := range actions {
		result[action.ID] = action
	}
	return result
}
