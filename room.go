package main

import "errors"

type Room struct {
	Id    string
	Owner string
	RoomSettings
}

type RoomSettings struct {
	Mode       MinesweeperMode
	Difficulty MinesweeperDifficulty
	H          uint
	W          uint
	N          uint
}

func (s *Server) RoomMembers(r *Room) (members []*Client) {
	members = make([]*Client, 0)
	s.cmutex.RLock()
	defer s.cmutex.RUnlock()
	for _, c := range s.clients {
		if c.CurrentRoom == r {
			members = append(members, c)
		}
	}
	return members
}

func (s *Server) handleRoomUpdate(c *Client, m *RoomUpdateMessage) {
	r := c.CurrentRoom
	if r.Owner != c.Username {
		c.SendMessage(&Message{SrvError: errors.New("modify room without permission")})
		return
	}
	if m.Settings != nil {
		r.RoomSettings = *m.Settings
	}

	us := &UserSyncMessage{make(map[string]*Client), true}
	for _, member := range s.RoomMembers(c.CurrentRoom) {
		us.Presences[member.Username] = member
	}
	s.Broadcast(&Message{UserSync: us})
}
