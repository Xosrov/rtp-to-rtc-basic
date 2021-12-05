// functions for serving websocket server
package main

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

func start_websocket(context **gin.Context, from_rtc <-chan Message, to_rtc chan<- Message, ended chan<- bool) {
	// start a websocket server for each user
	// receives a channel for communication with webrtc server
	upgrader := websocket.Upgrader{}
	sock, err := upgrader.Upgrade((*context).Writer, (*context).Request, nil)
	if err != nil {
		panic(err)
	}
	// start reading from ws and write outputs to channel,
	// this will be used in select to do required actions
	sock_message_reader := make(chan Message, 10)
	go func() {
		for {
			_, message, err := sock.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseGoingAway) {
					sock_message_reader <- Message{Status: WS_CLOSED}
					return
				}
				panic(err)
			}
			parsedMsg := Message{}
			if err := json.Unmarshal(message, &parsedMsg); err != nil {
				fmt.Println("Unknown message type received")
				continue
			}
			sock_message_reader <- parsedMsg
		}
	}()
	// close properly
	defer func() {
		sock.Close()
		fmt.Println("Closed socket")
		ended <- true
	}()

	// wait for action
	for {
		select {
		case message := <-from_rtc:
			// received close message from rtc
			if message.Status == RTC_CLOSED {
				fmt.Println("RTC closed, exiting Socket") //TODO: shouldn't do this, if RTC disconnects, maybe look for a way to reconnect
				return
			} else if message.Status == RTC_FAILED {
				fmt.Println("RTC failed, exiting Socket") // same as before
				return
			}
			// send message using websocket
			json, err := json.Marshal(&message)
			if err != nil {
				panic(err)
			}
			if err = sock.WriteMessage(websocket.TextMessage, json); err != nil {
				panic(err)
			}
		case message := <-sock_message_reader:
			// received close message from websock
			if message.Status == WS_CLOSED {
				fmt.Println("Socket reader closed, exiting Socket")
				// send close signal to rtc too
				to_rtc <- Message{Status: WS_CLOSED}
				return
			} else if message.Status == WS_STATUSFAILED {
				fmt.Println("Failed response from message, discarding it(change this behavior)")
				continue
			}
			// read message from websocket
			switch message.Type {
			case StringType:
				var message_str string
				if err := json.Unmarshal(message.Response, &message_str); err != nil {
					fmt.Println("Bad format for string message")
					continue
				}
				fmt.Printf("Got string: \"%s\"\n", message.Response)
			case SDPJsonType:
				sdp := webrtc.SessionDescription{}
				if err := json.Unmarshal(message.Response, &sdp); err != nil {
					fmt.Println("Bad format for sdp message")
					continue
				}
				// forward message to rtc only if correct format
				to_rtc <- message
			case ICECandidateType:
				iceCan := webrtc.ICECandidateInit{}
				if err := json.Unmarshal(message.Response, &iceCan); err != nil {
					fmt.Println("Bad format for ICE candidate")
					continue
				}
				// forward message to rtc only if correct format
				to_rtc <- message
			}
		}
	}

}
