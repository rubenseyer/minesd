package main

import (
	"sync/atomic"
	"errors"
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

func (c *Client) readloop() {
	for {
		if atomic.LoadInt32(&c.disconnected) == 1 {
			return
		}

		msg, err := c.ReceiveMessage()
		if err != nil {
			// TODO handle error/close
			c.Disconnect(nil)
			return
		}

		c.recv <- msg
	}
}

func (c *Client) writeloop() {
	for {
		msg, ok := <-c.send
		if !ok {
			return
		}

		err := c.SendMessage(msg)
		if err != nil {
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
	if msg != nil {
		_ = c.SendMessage(msg)
	}
	c.leave<-c
	_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
	return c.conn.Close()
}
