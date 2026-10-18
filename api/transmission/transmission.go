package transmission

import (
	"context"
	"sync"

	"github.com/hekmon/transmissionrpc/v2"

	"torrentino/common"
)

var (
	rpcClient     *transmissionrpc.Client
	rpcClientOnce sync.Once
	rpcClientErr  error
)

// client builds the RPC client on first use instead of during package
// initialization: the config is loaded from main(), so a client created in
// init() would point at an empty host. The construction error is returned to
// the caller rather than terminating the process, which also keeps the package
// importable from tests.
func client() (*transmissionrpc.Client, error) {
	rpcClientOnce.Do(func() {
		/* todo: transmissionrpc/v3
		endpoint, err := url.Parse("http://" + trn.Host + ":" + strconv.Itoa(trn.Port) + "/transmission/rpc")
		if err != nil {
		    log.Fatal(err)
		}
		rpcClient, rpcClientErr = transmissionrpc.New(endpoint, nil)
		*/
		rpcClient, rpcClientErr = transmissionrpc.New(common.Settings.Transmission.Host, "rpcuser", "rpcpass", nil)
	})
	return rpcClient, rpcClientErr
}

// The context is passed through to transmissionrpc, so an HTTP request that
// the browser abandoned also stops waiting for the RPC call.
func AddContext(ctx context.Context, torrentUrlOrMagnet string, downloadDir string) (torrent transmissionrpc.Torrent, err error) {
	c, err := client()
	if err != nil {
		return torrent, err
	}
	return c.TorrentAdd(ctx, transmissionrpc.TorrentAddPayload{
		Filename:    &torrentUrlOrMagnet,
		DownloadDir: &downloadDir,
	})
}

func Add(torrentUrlOrMagnet string, downloadDir string) (transmissionrpc.Torrent, error) {
	return AddContext(context.Background(), torrentUrlOrMagnet, downloadDir)
}

func DeleteContext(ctx context.Context, torrentId int64) (err error) {
	c, err := client()
	if err != nil {
		return err
	}
	return c.TorrentRemove(ctx, transmissionrpc.TorrentRemovePayload{
		IDs:             []int64{torrentId},
		DeleteLocalData: true,
	})
}

func Delete(torrentId int64) error {
	return DeleteContext(context.Background(), torrentId)
}

func StartContext(ctx context.Context, torrentId int64) (err error) {
	c, err := client()
	if err != nil {
		return err
	}
	return c.TorrentStartIDs(ctx, []int64{torrentId})
}

func Start(torrentId int64) error {
	return StartContext(context.Background(), torrentId)
}

func PauseContext(ctx context.Context, torrentId int64) (err error) {
	c, err := client()
	if err != nil {
		return err
	}
	return c.TorrentStopIDs(ctx, []int64{torrentId})
}

func Pause(torrentId int64) error {
	return PauseContext(context.Background(), torrentId)
}

func ListContext(ctx context.Context) (*[]transmissionrpc.Torrent, error) {
	c, err := client()
	if err != nil {
		return nil, err
	}
	t, err := c.TorrentGetAll(ctx)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func List() (*[]transmissionrpc.Torrent, error) {
	return ListContext(context.Background())
}
