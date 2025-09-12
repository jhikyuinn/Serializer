package types

type FabricChannel struct {
	Type           int               `bson:"type" json:"type"`
	ChannelId      string            `bson:"channel" json:"channel"`
	BlockNumber    int               `bson:"block" json:"block"`
	Transactions   int               `bson:"tx" json:"tx"`
	Members        int               `bson:"members" json:"members"`
	FabricPeers    []PeerAttribution `bson:"attr" json:"attr"`
	ChannelTrigger chan string
}

type PeerAttribution struct {
	Id   string
	Sent int
}
