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
	if !c.IsOwner() {
		c.send <- &Message{SrvError: "modify room without permission"}
		return
	}
	if m.Settings != nil {
		r.RoomSettings = *m.Settings
	}

	us := &UserSyncMessage{make(map[string]*Client), true}
	for _, member := range s.RoomMembers(r) {
		us.Presences[member.Username] = member
	}
	s.Broadcast(&Message{UserSync: us})
}

func (s *Server) handleRoomP2P(c *Client, m *RoomP2PMessage) {
	var us *UserSyncMessage

	if m.Username == "" {
		// No P2P target means leave
		oldroom := c.CurrentRoom
		c.CurrentRoom = s.newRoom(c, c.CurrentRoom.RoomSettings)
		log.Printf("<%v> joined [%v], left [%v]", c.Username, c.CurrentRoom.Id, oldroom.Id)
		us = s.resyncRoomOnLeave(oldroom, c)
		us.Presences[c.Username] = c
		s.Broadcast(&Message{UserSync: us})
		return
	}

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

	// Update room status if host answered p2p
	if m.Answer != "" && c.IsOwner() {
		// handle if room becomes empty, or owner leaves (resync the state)
		oldroom := target.CurrentRoom
		target.CurrentRoom = c.CurrentRoom
		log.Printf("<%v> joined [%v], left [%v]", target.Username, m.RoomId, oldroom.Id)
		us = s.resyncRoomOnLeave(oldroom, target)
		us.Presences[target.Username] = target
		s.Broadcast(&Message{UserSync: us})
	}

	target.send <- &Message{RoomP2P: m}
}

func (s *Server) newRoom(owner *Client, settings RoomSettings) *Room {
	id := s.roomid.MustGenerate()
	r := &Room{id, owner.Username, settings}
	s.rmutex.Lock()
	s.rooms[id] = r
	owner.CurrentRoom = r
	s.rmutex.Unlock()
	log.Printf("Room [%v] created (owned by <%v>)", r.Id, r.Owner)
	return r
}

func (s *Server) resyncRoomOnLeave(oldroom *Room, leaver *Client) (us *UserSyncMessage) {
	us = &UserSyncMessage{make(map[string]*Client), true}
	if oldroom.Owner == leaver.Username {
		var newowner *Client
		for _, member := range s.RoomMembers(oldroom) {
			if member == leaver {
				continue
			}
			if newowner == nil {
				newowner = member
			}
			// also broadcast users with changed presences due to room change in message
			us.Presences[member.Username] = member
		}
		if newowner != nil { // change room owner if room not empty
			oldowner := oldroom.Owner
			oldroom.Owner = newowner.Username
			log.Printf("Room [%v] changed owner to <%v> from <%v>", oldroom.Id, newowner.Username, oldowner)
		} else { // kill room if empty
			s.rmutex.Lock()
			delete(s.rooms, oldroom.Id)
			s.rmutex.Unlock()
			log.Printf("Room [%v] expired.", oldroom.Id)
		}
	}
	return us
}
