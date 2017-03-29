package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"
)

type responseSelf struct {
	ID string `json:"id"`
}

type responseRtmStart struct {
	Ok    bool         `json: "ok"`
	Error string       `json: "error"`
	URL   string       `json: "url"`
	Self  responseSelf `json: "self"`
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	token := "MYRTMTOKEN"
	url := fmt.Sprintf("https://slack.com/api/rtm.start?token=%s", token)

	resp, err := http.Get(url)

	checkErr(err)

	body, err := ioutil.ReadAll(resp.Body)

	checkErr(err)

	var respObj responseRtmStart

	err = json.Unmarshal(body, &respObj)

	fmt.Printf("%v\n", respObj)

	if respObj.Error != "" {
		panic(fmt.Sprintf("Error communicating with slack: %s", respObj.Error))
	}

	loop(respObj.URL, respObj.Self.ID)
}

type Message struct {
	ID      uint64 `json:"id"`
	Type    string `json:"type"`
	Channel string `json:"channel"`
	User    string `json:"user"`
	Text    string `json:"text"`
}

func getMessage(ws *websocket.Conn) (m Message, err error) {
	err = websocket.JSON.Receive(ws, &m)
	return m, err
}

var counter uint64

func nextID() uint64 {
	return atomic.AddUint64(&counter, 1)
}

func postMessage(ws *websocket.Conn, m Message) error {
	m.ID = nextID()
	return websocket.JSON.Send(ws, m)
}

func loop(url string, selfID string) {
	counter = 1

	ws, err := websocket.Dial(url, "", "https://api.slack.com/")
	checkErr(err)

	message, err := getMessage(ws)
	checkErr(err)

	if message.Type != "hello" {
		panic("Message was not hello, whoot?")
	}

	inChannel := make(chan Message)
	outChannel := make(chan Message)

	go func() {
		for {
			message, err := getMessage(ws)

			checkErr(err)

			inChannel <- message
		}
	}()

	go func() {
		for {
			message := <-outChannel
			fmt.Printf("Outgoing message: %v\n", message)

			err = postMessage(ws, message)

			checkErr(err)
		}
	}()

	go func() {
		for {
			message := <-inChannel
			fmt.Printf("Incoming message: %v\n", message)
			processMessage(message, outChannel, selfID)
		}
	}()

	for {
		pingMessage := Message{ID: 0, Type: "ping", Channel: "", Text: ""}

		outChannel <- pingMessage

		time.Sleep(time.Second * 15)
	}

}

func processMessage(message Message, replyChannel chan Message, selfID string) {
	dmPrefix := regexp.MustCompile(fmt.Sprintf(`<@%s>:?\s*`, selfID))

	if dmPrefix.MatchString(message.Text) {
		go processDMMessage(message, replyChannel, selfID, dmPrefix)
	} else {
		go processNonDMMessage(message, replyChannel)
	}
}

func processDMMessage(message Message, replyChannel chan Message, selfID string, dmPrefix *regexp.Regexp) {
	fmt.Printf("RECEIVED DM: %v\n", message)
	text := dmPrefix.ReplaceAllString(message.Text, "")
	var reply string

	switch {
	case strings.Contains(text, "hello"):
		reply = "hi"
	default:
		reply = "Dunno what to do!?"
	}

	replyMessage := Message{ID: 0, Type: "message", Channel: message.Channel, Text: reply}
	replyChannel <- replyMessage
}

func processNonDMMessage(message Message, replyChannel chan Message) {
	text := strings.Trim(message.Text, " ")

	var reply string

	switch {
	case text == "!help":
		reply = "HELP MESSAGE"
	}

	if len(reply) > 0 {
		replyChannel <- Message{ID: 0, Type: "message", Channel: message.Channel, Text: reply}
	}
}
