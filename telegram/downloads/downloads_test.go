package downloads

import (
	"context"
	"strings"
	"testing"

	"github.com/hekmon/transmissionrpc/v2"

	"torrentino/core"
)

func TestLineUsesPresentation(t *testing.T) {
	name := "Movie.mkv"
	size := int64(1024 * 1024)
	percent := 1.0
	ratio := 1.5
	peers := int64(3)

	p := NewPaginator(context.Background(), nil, nil)
	p.Alloc(1)
	p.Append(&core.DownloadItem{
		Torrent: transmissionrpc.Torrent{
			Name:               &name,
			DownloadedEver:     &size,
			PercentDone:        &percent,
			UploadRatio:        &ratio,
			PeersGettingFromUs: &peers,
		},
		Ext:    ".mkv",
		Status: "seeding",
	})

	line := p.Line(0)
	for _, want := range []string{"🎬Movie.mkv", "[100%]", "[1.50x]", "[seeding:3p]"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q lacks %q", line, want)
		}
	}
}
