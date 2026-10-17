package core

import (
	"context"
	"fmt"

	"torrentino/api/torrserver"
)

// TorrserverItem is one entry of the Torrserver list.
type TorrserverItem = torrserver.TSListItem

// TorrserverList returns the current Torrserver list.
func (s *Service) TorrserverList(ctx context.Context) ([]*TorrserverItem, error) {
	ctx, cancel := context.WithTimeout(ctx, serviceTimeout)
	defer cancel()
	result, err := s.Torrserver.List(ctx)
	if err != nil {
		return nil, Upstream(err)
	}
	items := make([]*TorrserverItem, 0, len(result))
	for i := range result {
		items = append(items, &result[i])
	}
	return items, nil
}

// TorrserverKey is the stable identity of a Torrserver entry.
func TorrserverKey(item *TorrserverItem) string {
	if item == nil {
		return ""
	}
	return "hash:" + item.Hash
}

// TorrserverCompare implements the only sort attribute of the screen.
func TorrserverCompare(a, b *TorrserverItem, attribute string) bool {
	if attribute == "Size" {
		return a.Torrent_Size < b.Torrent_Size
	}
	return false
}

// TorrserverActions returns the actions offered for a Torrserver entry.
func (s *Service) TorrserverActions(item *TorrserverItem) []Action {
	if item == nil {
		return nil
	}
	return []Action{{ID: "delete", Label: "delete"}}
}

// DoTorrserver removes a Torrserver record. Files on disk are not touched.
func (s *Service) DoTorrserver(ctx context.Context, item *TorrserverItem, action string) error {
	if item == nil {
		return ErrStaleItem
	}
	if action != "delete" {
		return fmt.Errorf("%w: %q", ErrNotAvailable, action)
	}
	ctx, cancel := context.WithTimeout(ctx, serviceTimeout)
	defer cancel()
	if err := s.Torrserver.Delete(ctx, item.Hash); err != nil {
		return Upstream(err)
	}
	return nil
}
