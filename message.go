package main

type Message struct {
	Sender   *Client `json:",omitempty"`
	SrvError error

	Hello      *HelloMessage      `json:",omitempty"`
	UserSync   *UserSyncMessage   `json:",omitempty"`
	RoomUpdate *RoomUpdateMessage `json:",omitempty"`
	Record     *RecordMessage     `json:",omitempty"`
	RecordSync *RecordSyncMessage `json:",omitempty"`
}

// HelloMessage is exchanged between server and client
// on first connect to indicate a successful connection.
type HelloMessage struct {
	Username string
	Settings *RoomSettings
}

// UserSyncMessage is exclusively sent by the server to
// indicate modifications in online presences (disconnect,
// room move...). This should not be used to sync current room
// state (that is done over WebRTC), but to update the list.
type UserSyncMessage struct {
	Presences map[string]*Client
	Partial   bool
}

// RoomUpdateMessage is sent by the client who owns the room to
// change the room settings reported to the server. This updates
// joined clients' presences. The client is responsible to sync
// this update over WebRTC to joined clients.
type RoomUpdateMessage struct {
	Settings *RoomSettings
}

// RecordMessage is sent by the client to indicate a
// game win. The server will both broadcast a RecordSyncMessage as
// well as update it's own game record.
type RecordMessage struct {
	Mode       MinesweeperMode
	Difficulty MinesweeperDifficulty
	Time       uint64
}

// RecordMessage is sent by the server to indicate a update
// to the recent or best games list.
type RecordSyncMessage struct {
	Best    map[MinesweeperDifficulty]*Record
	Latest  []*Record
	Partial bool
}
