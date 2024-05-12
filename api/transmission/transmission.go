package transmission

import (
	"context"
	"github.com/hekmon/transmissionrpc/v2"
	// "os"
	// "fmt"
	"torrentino/common"
)


var Transmission *transmissionrpc.Client

func Add(torrentUrlOrMagnet string) (torrent transmissionrpc.Torrent, err error) {
	torrent, err = Transmission.TorrentAdd(context.TODO(), transmissionrpc.TorrentAddPayload{
		Filename: &torrentUrlOrMagnet,
	})
	return
}

func Delete(torrentId int64) (err error) {
	err = Transmission.TorrentRemove(context.TODO(), transmissionrpc.TorrentRemovePayload{
		IDs: []int64{ torrentId },
		DeleteLocalData: true,
	})
	return
}

func List() (torrents []transmissionrpc.Torrent, err error) {
	return Transmission.TorrentGetAll(context.TODO())
}


func init() {
	var trn = &common.Settings.Transmission
	var err error
	/* todo: transmissionrpc/v3 
		endpoint, err := url.Parse("http://" + trn.Host + ":" + strconv.Itoa(trn.Port) + "/transmission/rpc")
		if err != nil {
		    panic(err)
		}
		Transmission, err = transmissionrpc.New(endpoint, nil)
	*/
	Transmission, err = transmissionrpc.New(trn.Host, "rpcuser", "rpcpass", nil)
	if err != nil {
		panic(err)
	}

}
