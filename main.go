// +build !js
// before running, execute this ffmpeg screen grabber:
// ffmpeg -re -f x11grab -draw_mouse 0 -show_region 1 -grab_x 0 -grab_y 185 -video_size 1920x870 -i :0 -c:v libvpx  -f rtp 'rtp://127.0.0.1:5004?pkt_size=1200'
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/rtcerr"
)

type MessageType uint

const (
	FAILED int = 0
	OK     int = 1
)
const (
	StringType       MessageType = iota // response is just a string
	SDPJsonType                         // response is a webrtc.SessionDescription object
	ICECandidateType                    // response is a webrtc.IceCandidate object
)

type WsMessage struct {
	Status   int             `json:"status"`
	Type     MessageType     `json:"type"`
	Response json.RawMessage `json:"response"`
}

var remoteSDP chan webrtc.SessionDescription = make(chan webrtc.SessionDescription, 1)
var IceCandidates chan webrtc.ICECandidateInit = make(chan webrtc.ICECandidateInit, 100) // store ice candidates that arrive before remote desc is set
var peerConnection *webrtc.PeerConnection
var upgrader = websocket.Upgrader{}
var incoming chan WsMessage = make(chan WsMessage, 100)
var outgoing chan WsMessage = make(chan WsMessage, 100)

func read_incoming_messages(sock *websocket.Conn) {
	for {
		_, message, err := sock.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseGoingAway) {
				return
			}
			panic(err)
		}
		parsedMsg := WsMessage{}
		if err := json.Unmarshal(message, &parsedMsg); err != nil {
			fmt.Println("Unknown message type received")
			continue
		}
		incoming <- parsedMsg
	}
}
func parse_incoming_messages() {
	for {
		parsedMsg := <-incoming
		switch parsedMsg.Type {
		case StringType:
			var message string
			if err := json.Unmarshal(parsedMsg.Response, &message); err != nil {
				fmt.Println("Bad format for string message")
				continue
			}
			fmt.Printf("Got string: \"%s\"\n", message)
		case SDPJsonType:
			sdp := webrtc.SessionDescription{}
			if err := json.Unmarshal(parsedMsg.Response, &sdp); err != nil {
				fmt.Println("Bad format for sdp message")
				continue
			}
			remoteSDP <- sdp
		case ICECandidateType:
			iceCan := webrtc.ICECandidateInit{}
			if err := json.Unmarshal(parsedMsg.Response, &iceCan); err != nil {
				fmt.Println("Bad format for ICE candidate")
				continue
			}
			if err := peerConnection.AddICECandidate(iceCan); err != nil {
				if errors.Is(err, rtcerr.InvalidStateError{Err: webrtc.ErrNoRemoteDescription}.Err) {
					IceCandidates <- iceCan
					continue
				}
				panic(err)
			}
			fmt.Println("ice candidaate set")
		}
	}
}
func write_outgoing_messages(sock *websocket.Conn) {
	// currently only writes sdp of server
	for {
		parsedMsg := <-outgoing
		json, err := json.Marshal(&parsedMsg)
		if err != nil {
			panic(err)
		}
		sock.WriteMessage(websocket.TextMessage, json)
	}
}
func start_websocket_rtc(c **gin.Context) {
	sock, err := upgrader.Upgrade((*c).Writer, (*c).Request, nil)
	if err != nil {
		panic(err)
	}
	defer func() {
		sock.Close()
	}()
	// do this better, create a channel to manage all at once
	go parse_incoming_messages()
	go write_outgoing_messages(sock)
	go start_backend()
	read_incoming_messages(sock) // read until closed

}
func serve(port string, restart chan bool) {
	router := gin.Default()
	router.Static("/static", "./view")
	router.LoadHTMLGlob("view/html/*")
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})
	router.GET("/ws", func(c *gin.Context) {
		fmt.Println(c.Request.URL.Query().Get("token"))
		fmt.Println("started socket and rtc ==============")
		start_websocket_rtc(&c)
		for len(incoming) > 0 {
			<-incoming
		}
		for len(outgoing) > 0 {
			<-outgoing
		}
		for len(remoteSDP) > 0 {
			<-remoteSDP
		}
		for len(IceCandidates) > 0 {
			<-IceCandidates
		}
		fmt.Println("stopped socket and rtc ==============")
	})
	router.Run(port)

}
func readPacket(Track *webrtc.TrackLocalStaticRTP, Listener *net.UDPConn) {
	inboundRTPPacket := make([]byte, 1600)
	for {
		n, _, err := Listener.ReadFrom(inboundRTPPacket)
		if err != nil {
			return
		}
		if _, err = Track.Write(inboundRTPPacket[:n]); err != nil {
			if errors.Is(err, io.ErrClosedPipe) {
				// connection closed
				return
			}
			panic(err)
		}
	}
}
func set_ice_candidates() {
	// set candidates in IceCandidates chan after remote desc is set
	for candidate := range IceCandidates {
		if err := peerConnection.AddICECandidate(candidate); err != nil {
			return
		}
	}
}
func start_backend() {
	// send stop signal if exit
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	peerConnection = pc // set to global peerconnection object
	if err != nil {
		panic(err)
	}
	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		iceJson, err := json.Marshal(candidate.ToJSON())
		if err != nil {
			panic(err)
		}
		outgoing <- WsMessage{OK, ICECandidateType, iceJson}
	})
	// UDP video listener for RTP on port 5004
	Vlistener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5004})
	if err != nil {
		panic(err)
	}
	// UDP audio listener for RTP on port 5005
	Alistener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5005})
	if err != nil {
		panic(err)
	}
	// make sure listeners and peer connection are closed after done
	defer func() {
		if err = Vlistener.Close(); err != nil {
			panic(err)
		}
		if err = Alistener.Close(); err != nil {
			panic(err)
		}
		if err = peerConnection.Close(); err != nil {
			panic(err)
		}
	}()

	// video track
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "vid")
	if err != nil {
		panic(err)
	}
	// audio track
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "aud")
	if err != nil {
		panic(err)
	}
	// add vide track to lc
	rtpVSender, err := peerConnection.AddTrack(videoTrack)
	if err != nil {
		panic(err)
	}
	// add audio track to lc
	rtpASender, err := peerConnection.AddTrack(audioTrack)
	if err != nil {
		panic(err)
	}
	// keep reading from tracks unless error occurs
	// video
	go func() {
		rtcpBuffer := make([]byte, 1500)
		for {
			if _, _, err := rtpVSender.Read(rtcpBuffer); err != nil {
				return
			}
		}
	}()
	// audio
	go func() {
		rtcpBuffer := make([]byte, 1500)
		for {
			if _, _, err := rtpASender.Read(rtcpBuffer); err != nil {
				return
			}
		}
	}()
	// get offer
	offer := <-remoteSDP
	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		panic(err)
	}
	go set_ice_candidates() // set ice candidates received before remote desc set
	// create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}
	// Sets the LocalDescription, and starts our UDP listeners
	if err = peerConnection.SetLocalDescription(answer); err != nil {
		panic(err)
	}
	// set local sdp
	sdpJson, err := json.Marshal(peerConnection.LocalDescription())
	if err != nil {
		panic(err)
	}
	outgoing <- WsMessage{OK, SDPJsonType, sdpJson}
	// send RTP packets forever
	go readPacket(videoTrack, Vlistener)
	readPacket(audioTrack, Alistener)
}

func main() {
	restarted := make(chan bool, 1)
	go serve(":8080", restarted)
	fmt.Println("started")
	wait := make(chan bool, 1)
	<-wait
}
