package core

import "torrentino/common/list"

// Screen descriptors are shared by both interfaces: a sort attribute or filter
// added here appears in Telegram and in the web interface at the same time.

func FindSorting() []list.Sorting {
	return []list.Sorting{
		{Attribute: "Size", Alias: "size", Order: 1},
		{Attribute: "Seeders", Alias: "seeds", Order: 1},
		{Attribute: "Peers", Alias: "peers", Order: 0},
		{Attribute: "Link", Alias: "file", Order: 0},
	}
}

func FindFilters() []string {
	return []string{"TrackerId"}
}

func DownloadsSorting() []list.Sorting {
	return []list.Sorting{
		{Attribute: "AddedDate", Alias: "date", Order: 1},
		{Attribute: "Name", Alias: "name", Order: 1},
		{Attribute: "DownloadedEver", Alias: "size", Order: 0},
		{Attribute: "IsDir", Alias: "dir", Order: 0},
	}
}

func DownloadsFilters() []string {
	return []string{"Status"}
}

func TorrserverSorting() []list.Sorting {
	return []list.Sorting{
		{Attribute: "Size", Alias: "size", Order: 1},
	}
}

func TorrserverFilters() []string {
	return nil
}
