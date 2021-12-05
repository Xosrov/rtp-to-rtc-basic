// functions for serving RTC server
package main

import (
	"encoding/json"
	"fmt"

	"github.com/pion/webrtc/v3"
)

func start_rtc(from_ws <-chan Message, to_ws chan<- Message, ended chan<- bool) {
	// send stop signal if exit
	serverConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	rtc_connection_state := make(chan webrtc.ICEConnectionState, 10)
	serverConnection.OnICEConnectionStateChange(func(is webrtc.ICEConnectionState) {
		rtc_connection_state <- is
	})
	// trickle ICE to websocket upon receiving
	serverConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		iceJson, err := json.Marshal(candidate.ToJSON())
		if err != nil {
			panic(err)
		}
		to_ws <- Message{Type: ICECandidateType, Status: WS_STATUSOK, Response: iceJson}
	})
	// add video track to connection
	rtpVSender, err := serverConnection.AddTrack(videoTrack)
	if err != nil {
		panic(err)
	}
	// add audio track to connection
	rtpASender, err := serverConnection.AddTrack(audioTrack)
	if err != nil {
		panic(err)
	}
	// make sure peer connection and rtp senders are closed after done
	defer func() {
		if err = serverConnection.Close(); err != nil {
			panic(err)
		}
		if err = rtpASender.Stop(); err != nil {
			panic(err)
		}
		if err = rtpVSender.Stop(); err != nil {
			panic(err)
		}
		fmt.Println("Closed RTC")
		ended <- true
	}()
	// keep sending from tracks to clients until rtpSenders closed
	// video
	go func() {
		rtcpBuffer := make([]byte, RTCBufferSize)
		for {
			if _, _, err := rtpVSender.Read(rtcpBuffer); err != nil {
				return
			}
		}
	}()
	// audio
	go func() {
		rtcpBuffer := make([]byte, RTCBufferSize)
		for {
			if _, _, err := rtpASender.Read(rtcpBuffer); err != nil {
				return
			}
		}
	}()
	// get offer, keep ice candidates for later
	// no errors should occur, because validation was done in websocket server
	var iceCandidatesBeforeSDP []webrtc.ICECandidateInit
	var offer = webrtc.SessionDescription{}
	rloop := true
	for rloop {
		message := <-from_ws
		switch message.Type {
		case SDPJsonType: // got sdp
			json.Unmarshal(message.Response, &offer)
			rloop = false
		case ICECandidateType: //got ice candidate
			iceCan := webrtc.ICECandidateInit{}
			json.Unmarshal(message.Response, &iceCan)
			iceCandidatesBeforeSDP = append(iceCandidatesBeforeSDP, iceCan)
		}
	}
	if err = serverConnection.SetRemoteDescription(offer); err != nil {
		panic(err)
	}
	// set ice candidates gotten before sdp set
	go func() {
		for _, iceCandidate := range iceCandidatesBeforeSDP {
			if err := serverConnection.AddICECandidate(iceCandidate); err != nil {
				fmt.Println("Error adding ice candidate")
			}
		}
	}()
	// create answer
	answer, err := serverConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}
	// Sets the LocalDescription, and starts our UDP listeners
	if err = serverConnection.SetLocalDescription(answer); err != nil {
		panic(err)
	}
	// get local sdp and send to websocket
	sdpJson, err := json.Marshal(serverConnection.LocalDescription())
	if err != nil {
		panic(err)
	}
	to_ws <- Message{Type: SDPJsonType, Status: WS_STATUSOK, Response: sdpJson}

	// wait for action
	for {
		select {
		case message := <-from_ws:
			if message.Status == WS_CLOSED {
				fmt.Println("Socket closed, exiting RTC")
				return
			}
			if message.Type == ICECandidateType {
				iceCan := webrtc.ICECandidateInit{}
				json.Unmarshal(message.Response, &iceCan)
				if err := serverConnection.AddICECandidate(iceCan); err != nil {
					fmt.Println("Error adding ice candidate")
				}
			}
		case message := <-rtc_connection_state:
			if message == webrtc.ICEConnectionStateFailed { //TODO: shouldn't do this, if RTC disconnects, maybe look for a way to reconnect
				fmt.Println("RTC Failed, exiting RTC")
				to_ws <- Message{Status: RTC_FAILED}
				return
			} else if message == webrtc.ICEConnectionStateClosed { // same as above
				fmt.Println("RTC Closed, exiting RTC")
				to_ws <- Message{Status: RTC_CLOSED}
				return
			}
		}
	}
}
