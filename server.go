package main

import (
	"container/ring"
	"encoding/json"
	"html"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"fmt"

	"net/url"

	"github.com/gorilla/websocket"
	"github.com/teris-io/shortid"
)

var upgrader = &websocket.Upgrader{
	HandshakeTimeout: 20 * time.Second,
	Subprotocols:     []string{"mines"},
	CheckOrigin: func(r *http.Request) bool {
		if FlagOrigin {
			return true
		}
		origin := r.Header["Origin"]
		if len(origin) == 0 {
			return true
		}
		u, err := url.Parse(origin[0])
		if err != nil {
			return false
		}
		if strings.ToLower(u.Host) == strings.ToLower(r.Host) {
			return true
		}
		if u.Host == "rubenseyer.github.io" {
			return true
		}
		return false
	},
}

type Server struct {
	clients map[string]*Client
	cmutex  sync.RWMutex
	rooms   map[string]*Room
	rmutex  sync.RWMutex
	roomid  *shortid.Shortid

	addr     string
	listener net.Listener
	certFile string
	keyFile  string

	recordsLatest   *ring.Ring
	recordsBestSolo map[MinesweeperDifficulty]*Record
	recordsFile     string

	ch    chan *Message
	leave chan *Client
	done  chan struct{}
	wg    sync.WaitGroup

	statsticker *time.Ticker
}

func (s *Server) loop() {
	defer s.wg.Done()
	for {
		select {
		case client := <-s.leave:
			s.handleDisconnect(client)
		case <-s.done:
			return
		case msg := <-s.ch:
			if msg.Hello != nil {
				s.handleNewClient(msg.Sender, msg.Hello)
			} else if msg.RoomUpdate != nil {
				s.handleRoomUpdate(msg.Sender, msg.RoomUpdate)
			} else if msg.RoomP2P != nil {
				s.handleRoomP2P(msg.Sender, msg.RoomP2P)
			} else if msg.Record != nil {
				s.handleRecord(msg.Sender, msg.Record)
			}
			if msg.Chat != "" {
				log.Printf("<%v> '%v'", msg.Sender.Username, msg.Chat)
				s.Broadcast(&Message{Sender: msg.Sender, Chat: msg.Chat})
			}
		case <-s.statsticker.C:
			s.cmutex.RLock()
			s.rmutex.RLock()
			if len(s.clients) > 0 {
				log.Printf("Currently %v clients online in %v rooms", len(s.clients), len(s.rooms))
			}
			s.SaveRecords()
			s.rmutex.RUnlock()
			s.cmutex.RUnlock()
		}
	}
}

func (s *Server) handleNewClient(c *Client, m *HelloMessage) {
	if m.Room == nil {
		log.Printf("Failed to initialize new client: no settings sent")
		_ = c.Disconnect(&Message{SrvError: "missing settings"})
		return
	}
	if c.Username != html.EscapeString(c.Username) {
		log.Printf("Failed to initialize new client: unsafe username")
		_ = c.Disconnect(&Message{SrvError: "invalid username"})
		return
	}

	s.cmutex.Lock()
	if _, ok := s.clients[c.Username]; ok {
		log.Printf("Failed to initialize new client: username taken")
		c.Username = "" // otherwise we panic.. duh
		_ = c.Disconnect(&Message{SrvError: "username taken"})
		s.cmutex.Unlock()
		return
	} else {
		s.clients[c.Username] = c
		s.cmutex.Unlock()
	}

	log.Printf("<%v> Connected.", c.Username)

	c.wg.Add(1)     // sync writer release
	go c.readloop() // reads cannot be concurrent either, but they are localized anyway
	go c.writeloop()

	// TODO create client room, more data?
	s.rmutex.RLock()
	r, ok := s.rooms[m.Room.Id]
	s.rmutex.RUnlock()
	if ok {
		c.CurrentRoom = r
	} else {
		m.Room = s.newRoom(c, m.Room.RoomSettings)
	}
	syncmsg := &Message{Chat: "Welcome to the MINES server v0.2!", Hello: m}
	syncmsg.UserSync = s.generateUserSync(c)
	syncmsg.RecordSync = s.generateRecordsSync()
	c.send <- syncmsg

	// broadcast connect
	s.BroadcastExcept(&Message{
		Chat:     fmt.Sprintf("%v connected to the server.", c.Username),
		UserSync: &UserSyncMessage{map[string]*Client{c.Username: c}, true},
	}, c)
}

func (s *Server) generateUserSync(receiver *Client) (us *UserSyncMessage) {
	s.cmutex.RLock()
	defer s.cmutex.RUnlock()
	us = &UserSyncMessage{Presences: make(map[string]*Client)}
	for k, v := range s.clients {
		if v == receiver {
			continue
		}
		us.Presences[k] = v
	}
	return
}

func (s *Server) handleDisconnect(c *Client) {
	log.Printf("<%v> Disconnected.", c.Username)
	s.cmutex.Lock()
	_, ok := s.clients[c.Username]
	delete(s.clients, c.Username)
	s.cmutex.Unlock()

	if !ok {
		return // Ignore if connection was not complete.
	}

	us := s.resyncRoomOnLeave(c.CurrentRoom, c)
	us.Presences[c.Username] = nil
	s.BroadcastExcept(&Message{
		Chat:     fmt.Sprintf("%v disconnected from the server.", c.Username),
		UserSync: us,
	}, c)
}

func (s *Server) Broadcast(msg *Message) {
	s.cmutex.RLock()
	defer s.cmutex.RUnlock()

	for _, c := range s.clients {
		select {
		case c.send <- msg:
		default:
		}
	}
}
func (s *Server) BroadcastExcept(msg *Message, not *Client) {
	s.cmutex.RLock()
	defer s.cmutex.RUnlock()

	for _, c := range s.clients {
		if c == not {
			continue
		}
		select {
		case c.send <- msg:
		default:
		}
	}
}

func (s *Server) Start() error {
	s.ch = make(chan *Message)
	s.leave = make(chan *Client, 10)
	s.done = make(chan struct{})

	s.clients = make(map[string]*Client)
	s.rooms = make(map[string]*Room)

	sid, err := shortid.New(1, shortid.DefaultABC, uint64(time.Now().UnixNano()))
	s.roomid = sid
	if err != nil {
		log.Fatalf("Failed to create room id generator: %v", err)
	}

	s.recordsLatest = ring.New(10)
	s.LoadRecords(s.recordsFile)

	s.statsticker = time.NewTicker(1 * time.Minute)

	if strings.HasPrefix(s.addr, "unix:") {
		path := strings.TrimPrefix(s.addr, "unix:")
		_ = os.Remove(path)
		l, err := net.Listen("unix", path)
		if err != nil {
			log.Fatalf("Listen failed: %v", err)
		}
		s.listener = l
	} else {
		l, err := net.Listen("tcp", s.addr)
		if err != nil {
			log.Fatalf("Listen failed: %v", err)
		}
		s.listener = l
	}
	log.Printf("Listening on %v", s.listener.Addr().String())

	go func() {
		var err error
		if s.certFile != "" && s.keyFile != "" {
			err = http.ServeTLS(s.listener, s, s.certFile, s.keyFile)
		} else {
			err = http.Serve(s.listener, s)
		}
		if err != http.ErrServerClosed && !isClosedErr(err) {
			log.Fatalf("Fatal HTTP server error: %v", err)
		}
	}()

	s.wg.Add(1)
	go s.loop()

	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Websocket upgrade failed: %v", err)
		return
	}

	c := &Client{
		conn:  conn,
		recv:  s.ch,
		send:  make(chan *Message),
		leave: s.leave,
	}

	if r.Header.Get("X-Real-IP") != "" {
		log.Printf("Received new connection from %v", r.Header.Get("X-Real-IP"))
	} else {
		log.Printf("Received new connection from %v", c.conn.RemoteAddr())
	}

	msg, err := c.ReceiveMessage() // do this here in case we have to wait
	if err != nil {
		log.Printf("Failed to initialize new client: message read err: %v", err)
		c.Disconnect(nil)
		return
	}
	if msg.Hello == nil {
		log.Println("Failed to initialize new client: not a hello message first")
		c.Disconnect(nil)
		return
	}
	c.Username = msg.Hello.Username

	// TODO maybe just call handleNewClient directly (or as goroutine)? perhaps the synced state is better
	s.ch <- msg
}

func (s *Server) Stop() {
	s.Broadcast(&Message{Chat: "Server is going down!"})

	// Stop the event loop
	close(s.done)
	s.wg.Wait()

	// TODO more cleanup?
	for _, c := range s.clients {
		_ = c.conn.Close()
	}
	s.listener.Close() // closing listener also kills http server

	s.SaveRecords()
}

func (s *Server) LoadRecords(recordsFile string) {
	if s.recordsFile != "" {
		recordsRaw, err := ioutil.ReadFile(s.recordsFile)
		if err != nil {
			log.Printf("Failed to read records file: %v", err)
			s.recordsFile = ""
		} else {
			err = json.Unmarshal(recordsRaw, &s.recordsBestSolo)
			if err != nil {
				log.Printf("Failed to parse records file: %v", err)
				s.recordsFile = ""
			}
		}
	}
	if s.recordsFile == "" {
		s.recordsBestSolo = make(map[MinesweeperDifficulty]*Record, 4)
	}
}

func (s *Server) SaveRecords() {
	if s.recordsFile == "" {
		return
	}
	recordsRaw, err := json.Marshal(s.recordsBestSolo)
	if err != nil {
		log.Printf("Failed to generate records file: %v", err)
	} else {
		err = ioutil.WriteFile(s.recordsFile, recordsRaw, 0666)
		if err != nil {
			log.Printf("Failed to write records file: %v", err)
		} else {
			log.Print("Records synched to disk.")
		}
	}
}

func isClosedErr(err error) bool {
	// Why is the stdlib like this?
	// https://github.com/golang/go/issues/4373
	if err != nil && strings.Contains(err.Error(), "use of closed network connection") {
		return true
	}
	return false
}
