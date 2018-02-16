package main

type Message struct {
	Sender   *Client `json:",omitempty"`
	SrvError string  `json:",omitempty"`
	Chat     string  `json:",omitempty"`

	Hello      *HelloMessage      `json:",omitempty"`
	UserSync   *UserSyncMessage   `json:",omitempty"`
	RoomUpdate *RoomUpdateMessage `json:",omitempty"`
	RoomP2P    *RoomP2PMessage    `json:",omitempty"`
	Record     *RecordMessage     `json:",omitempty"`
	RecordSync *RecordSyncMessage `json:",omitempty"`
}

// HelloMessage is exchanged between server and client
// on first connect to indicate a successful connection.
type HelloMessage struct {
	Username string
	Room     *Room
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

// RoomP2PMessage is sent by the client for WebRTC signaling and
// forwarded by the server to the client with the given username,
// flipping the field around to contain the sender's name.
// If the Username field is empty, the message is interpreted as
// leaving all available rooms.
type RoomP2PMessage struct {
	Username  string `json:",omitempty"`
	RoomId    string `json:",omitempty"`
	Offer     string `json:",omitempty"`
	Answer    string `json:",omitempty"`
	Candidate string `json:",omitempty"`
}

// RecordMessage is sent by the client to indicate a
// game win. The server will both broadcast a RecordSyncMessage as
// well as update it's own game record.
type RecordMessage struct {
	Mode       MinesweeperMode
	Difficulty MinesweeperDifficulty
	Time       uint64
}

// RecordSyncMessage is sent by the server to indicate a update
// to the recent or best games list.
type RecordSyncMessage struct {
	Best    map[MinesweeperDifficulty]*Record
	Latest  []*Record
	Partial bool
}
