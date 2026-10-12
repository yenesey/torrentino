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

func Add(torrentUrlOrMagnet string, downloadDir string) (torrent transmissionrpc.Torrent, err error) {
	c, err := client()
	if err != nil {
		return torrent, err
	}
	return c.TorrentAdd(context.TODO(), transmissionrpc.TorrentAddPayload{
		Filename:    &torrentUrlOrMagnet,
		DownloadDir: &downloadDir,
	})
}

func Delete(torrentId int64) (err error) {
	c, err := client()
	if err != nil {
		return err
	}
	return c.TorrentRemove(context.TODO(), transmissionrpc.TorrentRemovePayload{
		IDs:             []int64{torrentId},
		DeleteLocalData: true,
	})
}

func Start(torrentId int64) (err error) {
	c, err := client()
	if err != nil {
		return err
	}
	return c.TorrentStartIDs(context.TODO(), []int64{torrentId})
}

func Pause(torrentId int64) (err error) {
	c, err := client()
	if err != nil {
		return err
	}
	return c.TorrentStopIDs(context.TODO(), []int64{torrentId})
}

func List() (*[]transmissionrpc.Torrent, error) {
	c, err := client()
	if err != nil {
		return nil, err
	}
	t, err := c.TorrentGetAll(context.TODO())
	if err != nil {
		return nil, err
	}
	return &t, nil
}
