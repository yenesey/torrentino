package core

import (
	"context"

	"github.com/hekmon/transmissionrpc/v2"

	"torrentino/api/jackett"
	"torrentino/api/torrserver"
	"torrentino/api/transmission"
)

// JackettAPI is the narrow search interface core needs. Tests substitute a
// fake so they never touch the network or common.Settings.
type JackettAPI interface {
	Query(ctx context.Context, query string, indexers []string) ([]jackett.Result, error)
	GetInfoHash(ctx context.Context, link string) (string, error)
}

// TransmissionAPI covers the torrent client calls used by the screens.
type TransmissionAPI interface {
	List(ctx context.Context) ([]transmissionrpc.Torrent, error)
	Add(ctx context.Context, link string, downloadDir string) error
	Delete(ctx context.Context, id int64) error
	Start(ctx context.Context, id int64) error
	Pause(ctx context.Context, id int64) error
}

// TorrserverAPI covers the Torrserver calls used by the screens.
type TorrserverAPI interface {
	List(ctx context.Context) ([]torrserver.TSListItem, error)
	Add(ctx context.Context, link string, title string, poster string) error
	Delete(ctx context.Context, hash string) error
}

// Service is the domain entry point. The zero value is not usable; construct
// it with NewService or fill the interfaces (tests use fakes).
type Service struct {
	Jackett      JackettAPI
	Transmission TransmissionAPI
	Torrserver   TorrserverAPI
}

// NewService returns a service backed by the real API clients. The clients
// themselves keep reading common.Settings on every call, as before.
func NewService() *Service {
	return &Service{
		Jackett:      jackettAdapter{},
		Transmission: transmissionAdapter{},
		Torrserver:   torrserverAdapter{},
	}
}

// Default is the process-wide service used by the Telegram handlers and the
// web server. Tests construct their own Service instead of touching it.
var Default = NewService()

type jackettAdapter struct{}

func (jackettAdapter) Query(ctx context.Context, query string, indexers []string) ([]jackett.Result, error) {
	result, err := jackett.QueryContext(ctx, query, indexers)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return *result, nil
}

func (jackettAdapter) GetInfoHash(ctx context.Context, link string) (string, error) {
	return jackett.GetInfoHashContext(ctx, link)
}

type transmissionAdapter struct{}

func (transmissionAdapter) List(ctx context.Context) ([]transmissionrpc.Torrent, error) {
	result, err := transmission.ListContext(ctx)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return *result, nil
}

func (transmissionAdapter) Add(ctx context.Context, link string, downloadDir string) error {
	_, err := transmission.AddContext(ctx, link, downloadDir)
	return err
}

func (transmissionAdapter) Delete(ctx context.Context, id int64) error {
	return transmission.DeleteContext(ctx, id)
}

func (transmissionAdapter) Start(ctx context.Context, id int64) error {
	return transmission.StartContext(ctx, id)
}

func (transmissionAdapter) Pause(ctx context.Context, id int64) error {
	return transmission.PauseContext(ctx, id)
}

type torrserverAdapter struct{}

func (torrserverAdapter) List(ctx context.Context) ([]torrserver.TSListItem, error) {
	result, err := torrserver.ListContext(ctx)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return *result, nil
}

func (torrserverAdapter) Add(ctx context.Context, link string, title string, poster string) error {
	return torrserver.AddContext(ctx, link, title, poster)
}

func (torrserverAdapter) Delete(ctx context.Context, hash string) error {
	return torrserver.DeleteContext(ctx, hash)
}
