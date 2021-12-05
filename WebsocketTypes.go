// basic type declarations used by other modules
package main

import "encoding/json"

// status codes for websocket communication
const (
	// control values
	RTC_FAILED int = -3
	RTC_CLOSED int = -2
	WS_CLOSED  int = -1
	// response values
	WS_STATUSFAILED int = 0
	WS_STATUSOK     int = 1
)

// message types for websocket communication
type MessageType uint

// list of message types specified as numbers starting from 0
const (
	StringType       MessageType = iota // response is just a string
	SDPJsonType                         // response is a webrtc.SessionDescription object
	ICECandidateType                    // response is a webrtc.IceCandidate object
)

// struct describing a websocket message in general form
type Message struct {
	Type     MessageType     `json:"type"`     // required
	Status   int             `json:"status"`   //optional
	Response json.RawMessage `json:"response"` //optional
}
