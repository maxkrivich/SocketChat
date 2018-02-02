package main

import (
	"flag"
	"net"
	"fmt"
	"log"
	"bufio"
	"os"
	"encoding/json"
	"time"
	"strings"
	"io"
)

const (
	AlohaCommand      = 1 << iota
	LeaveCommand
	NewMessageCommand
	//FileTransferCommand
)

const MessageDelimiter byte = '\n'

type Message struct {
	MessageType int    `json:"type"`
	Content     string `json:"content"`
	Sender      string `json:"sender"`
	Timestamp   string `json:"timestamp"`
}

type Client struct {
	alive      bool
	connection net.Conn
	username   *string
	incoming   chan *Message
	outgoing   chan *Message
}

func newClient(conn net.Conn, username *string) *Client {
	client := &Client{
		connection: conn,
		username:   username,
		incoming:   make(chan *Message),
		outgoing:   make(chan *Message),
		alive:      true,
	}
	return client
}

func (c *Client) ReadNet() {
	reader := bufio.NewReader(c.connection)
	for c.alive {
		line, err := reader.ReadString(MessageDelimiter)
		if err != nil {
			if err.Error() == "EOF" {
				os.Exit(0)
			} else {
				log.Fatal(err)
			}
		}
		var msg Message
		err = json.Unmarshal([]byte(line), &msg)
		if err != nil {
			log.Println(err)
			continue
		}
		c.incoming <- &msg
	}
}

func (c *Client) ReadIO() {
	scan := bufio.NewScanner(os.Stdin)
	for scan.Scan() && c.alive {
		s := scan.Text()
		msg := Message{
			Content:     s,
			Sender:      *c.username,
			MessageType: NewMessageCommand,
			Timestamp:   time.Now().String(),
		}
		if strings.HasPrefix(s, ":file") {
			connection, err := net.Dial("tcp", fmt.Sprintf("%s:%d", *host, 8008))
			if err != nil {
				log.Fatal(err)
			}
			go c.sendFile(connection, s[len(":file")+1:])
			continue
		}

		if s == ":e" || s == ":exit" {
			c.alive = false
			msg.MessageType = LeaveCommand
		}
		c.outgoing <- &msg
	}
}

func (c *Client) sendFile(conn net.Conn, path string) error {
	file, err := os.Open(strings.TrimSpace(path))
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		log.Println(err)
		return err
	}
	conn.Write([]byte(fmt.Sprintf("%s %d\n", info.Name(), info.Size())))
	cnt, err := io.Copy(conn, file)
	if err != nil {
		log.Println(err)
		return err
	}
	log.Println(cnt, "bytes send")
	return nil
}

func (c *Client) WriteNet() {
	writer := bufio.NewWriter(c.connection)
	defer c.connection.Close()
	for c.alive {
		for m := range c.outgoing {
			b, err := json.Marshal(&m)
			if err != nil {
				log.Println(err)
				continue
			}
			jsonMessage := string(b)
			writer.WriteString(jsonMessage)
			writer.WriteByte(MessageDelimiter)
			writer.Flush()
		}
	}
}

func (c *Client) handleReceivedMessage() {
	for c.alive {
		for rm := range c.incoming {
			switch rm.MessageType {
			case NewMessageCommand:
				log.Printf("%s$~%s", rm.Sender, rm.Content)
			}
		}
	}
}

// cli arguments
var (
	username = flag.String("username", "", "Enter you username")
	host     = flag.String("host", "localhost", "Enter remote server address")
	port     = flag.Int("port", 8080, "Enter remote server port")
)

// Probably stupid solution
func main() {
	flag.Parse()

	if len(*username) == 0 {
		log.Fatal("Enter your username")
	}

	connection, err := net.Dial("tcp", fmt.Sprintf("%s:%d", *host, *port))
	if err != nil {
		log.Fatal(err)
	}

	client := newClient(connection, username)

	go client.ReadNet()
	go client.WriteNet()
	go client.handleReceivedMessage()

	client.outgoing <- &Message{MessageType: AlohaCommand, Sender: *username}
	client.ReadIO()
}
