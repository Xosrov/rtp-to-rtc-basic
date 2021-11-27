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
)

type MessageType uint

const (
	FAILED int = 0
	OK     int = 1
)
const (
	String       MessageType = iota // response is just a string
	SDPJson                         // response is a webrtc.SessionDescription object
	ICECandidate                    // response is a webrtc.IceCandidate object
)

type WsMessage struct {
	Status   int             `json:"status"`
	Type     MessageType     `json:"type"`
	Response json.RawMessage `json:"response"`
}

var stopped chan bool = make(chan bool, 1)
var remoteSDP chan webrtc.SessionDescription = make(chan webrtc.SessionDescription, 1)

var upgrader = websocket.Upgrader{}
var incoming chan WsMessage = make(chan WsMessage, 100)
var outgoing chan WsMessage = make(chan WsMessage, 100)

func read_incoming_messages(sock *websocket.Conn) {
	for {
		_, message, err := sock.ReadMessage()
		if err != nil {
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
		case String:
			var message string
			if err := json.Unmarshal(parsedMsg.Response, &message); err != nil {
				fmt.Println("Bad format for string message")
				return
			}
			fmt.Printf("Got string: \"%s\"\n", message)
		case SDPJson:
			sdp := webrtc.SessionDescription{}
			if err := json.Unmarshal(parsedMsg.Response, &sdp); err != nil {
				fmt.Println("Bad format for sdp message")
				return
			}
			select {
			case remoteSDP <- sdp:
			default:
				fmt.Println("sdp channel is full, currently supports only one client at a time.")
			}
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
func start_websocket(c **gin.Context) {
	sock, err := upgrader.Upgrade((*c).Writer, (*c).Request, nil)
	if err != nil {
		panic(err)
	}
	defer sock.Close()
	go read_incoming_messages(sock)
	go parse_incoming_messages()
	go write_outgoing_messages(sock)
	<-stopped

}
func serve(port string) {
	defer func() {
		stopped <- true
	}()
	router := gin.Default()
	router.Static("/static", "./view")
	router.LoadHTMLGlob("view/html/*")
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})
	router.GET("/ws", func(c *gin.Context) {
		start_websocket(&c)
	})
	router.Run(port)

}
func rtcServer() {
	// Manage starting RTC connections and restarted connections
	// run until stopped
	for len(stopped) == 0 {
		fmt.Println("running --- ---- ---- ")
		start_backend()
	}
}
func readPacket(Track *webrtc.TrackLocalStaticRTP, Listener *net.UDPConn, running *bool, res chan bool) {
	inboundRTPPacket := make([]byte, 1600)
	for *running {
		n, _, err := Listener.ReadFrom(inboundRTPPacket)
		if err != nil {
			panic(fmt.Sprintf("error during read: %s", err))
		}
		if _, err = Track.Write(inboundRTPPacket[:n]); err != nil {
			if errors.Is(err, io.ErrClosedPipe) {
				// connection closed
				return
			}
			panic(err)
		}
	}
	res <- true
}
func start_backend() {
	// send stop signal if exit
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		panic(err)
	}
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
	// make sure listeners are closed after done
	defer func() {
		if err = Vlistener.Close(); err != nil {
			panic(err)
		}
		if err = Alistener.Close(); err != nil {
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

	running := true
	// ICE handler
	peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		fmt.Println("Connection state changed: " + state.String())
		switch state {
		case webrtc.ICEConnectionStateFailed:
			if err := peerConnection.Close(); err != nil {
				panic(err)
			}
			running = false
		case webrtc.ICEConnectionStateDisconnected:
			running = false
		}

	})
	// get offer
	offer := <-remoteSDP
	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		panic(err)
	}
	// create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}
	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Sets the LocalDescription, and starts our UDP listeners
	if err = peerConnection.SetLocalDescription(answer); err != nil {
		panic(err)
	}
	// Block until ICE Gathering is complete, disabling trickle ICE
	<-gatherComplete
	// set local sdp
	sdpJson, err := json.Marshal(peerConnection.LocalDescription())
	if err != nil {
		panic(err)
	}
	outgoing <- WsMessage{OK, SDPJson, sdpJson}
	// send RTP packets forever
	vc := make(chan bool, 1)
	ac := make(chan bool, 1)
	go readPacket(videoTrack, Vlistener, &running, vc)
	go readPacket(audioTrack, Alistener, &running, ac)
	<-vc
	<-ac
}

func main() {
	go serve(":8080")
	go rtcServer()
	fmt.Println("started")
	<-stopped
}
