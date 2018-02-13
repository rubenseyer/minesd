package main

import (
	"log"
)

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
		c.send <- &Message{SrvError: "modify room without permission"}
		return
	}
	if m.Settings != nil {
		r.RoomSettings = *m.Settings
	}

	s.resyncRoom(r)
}

func (s *Server) handleRoomP2P(c *Client, m *RoomP2PMessage) {
	s.cmutex.RLock()
	target, ok := s.clients[m.Username]
	s.cmutex.RUnlock()
	if !ok {
		c.send <- &Message{SrvError: "invalid user in p2p message"}
		return
	}
	m.Username = c.Username
	if m.Offer != "" {
		if target.CurrentRoom.Id != m.RoomId {
			c.send <- &Message{SrvError: "invalid room in p2p message"}
			return
		}
		log.Printf("P2P offer  [%v] <%v>▶<%v>", m.RoomId, c.Username, target.Username)
	} else if m.Answer != "" {
		if c.CurrentRoom.Id != m.RoomId {
			c.send <- &Message{SrvError: "invalid room in p2p message"}
			return
		}
		log.Printf("P2P answer [%v] <%v>▶<%v>", m.RoomId, c.Username, target.Username)
	}

	// Update room status
	if m.Answer != "" {
		// todo handle if room becomes empty, or owner leaves (resync the state)
		target.CurrentRoom = c.CurrentRoom
		us := &UserSyncMessage{map[string]*Client{target.Username: target}, true}
		s.Broadcast(&Message{UserSync: us})
	}

	target.send <- &Message{RoomP2P: m}
}

func (s *Server) resyncRoom(room *Room) {
	us := &UserSyncMessage{make(map[string]*Client), true}
	for _, member := range s.RoomMembers(room) {
		us.Presences[member.Username] = member
	}
	s.Broadcast(&Message{UserSync: us})
}
