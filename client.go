package main

import (
	"errors"
	"log"
	"sync/atomic"

	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	conn *websocket.Conn
	recv chan *Message
	send chan *Message

	leave        chan *Client
	disconnected int32 // atomic

	Username    string
	CurrentRoom *Room
}

const pingfreq = 1 * time.Minute

func (c *Client) readloop() {
	c.conn.SetReadDeadline(time.Now().Add(pingfreq))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pingfreq))
		return nil
	})
	for {
		if atomic.LoadInt32(&c.disconnected) == 1 {
			return
		}

		msg, err := c.ReceiveMessage()
		if err != nil {
			// TODO handle error/close
			log.Printf("<%v> recv error: %v", c.Username, err)
			c.Disconnect(nil)
			return
		}

		c.recv <- msg
	}
}

func (c *Client) writeloop() {
	pinger := time.NewTicker(pingfreq - 10*time.Second)

	for {
		var err error
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			err = c.SendMessage(msg)
		case <-pinger.C:
			err = c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(pingfreq))
		}
		if err != nil {
			log.Printf("<%v> send error: %v", c.Username, err)
			return // an eventual error would be handled in the read loop
		}
	}
}

func (c *Client) SendMessage(msg *Message) error {
	if atomic.LoadInt32(&c.disconnected) == 1 {
		return errors.New("attempt to send to disconnected client")
	}
	return c.conn.WriteJSON(msg)
}

func (c *Client) ReceiveMessage() (msg *Message, err error) {
	if atomic.LoadInt32(&c.disconnected) == 1 {
		return nil, errors.New("attempt to receive from disconnected client")
	}
	msg = &Message{Sender: c}
	err = c.conn.ReadJSON(msg)
	return
}

func (c *Client) Disconnect(msg *Message) error {
	if !atomic.CompareAndSwapInt32(&c.disconnected, 0, 1) {
		return errors.New("client disconnected multiple times")
	}
	close(c.send)
	c.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	if msg != nil {
		_ = c.conn.WriteJSON(msg)
	}
	c.leave <- c
	_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
	return c.conn.Close()
}
