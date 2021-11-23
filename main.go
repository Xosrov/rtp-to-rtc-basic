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
	"github.com/pion/webrtc/v3"
)

var stopped chan bool = make(chan bool, 1)
var localSDP *webrtc.SessionDescription = nil
var remoteSDP chan webrtc.SessionDescription = make(chan webrtc.SessionDescription, 1)

func serve(port string) {
	router := gin.Default()
	router.Static("/static", "./view")
	router.LoadHTMLGlob("view/html/*")
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})
	router.POST("/sdp", func(c *gin.Context) {
		decoder := json.NewDecoder(c.Request.Body)
		answer := webrtc.SessionDescription{}
		decoder.Decode(&answer)
		select {
		case remoteSDP <- answer:
		default:
			fmt.Println("Channel is still not emptied, disregarding new request")
		}
		fmt.Println("Got client sdp")
		c.String(http.StatusOK, "OK")
	})
	router.GET("/sdp", func(c *gin.Context) {
		// wait for local sdp to be ready
		for localSDP == nil {
		}
		c.JSON(http.StatusOK, *localSDP)
		fmt.Println("Telling client sdp")
		// empty for next client, not the best way but works
		localSDP = nil
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

	var running bool = true
	// ICE handler
	peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		fmt.Println("Connection state changed: " + state.String())
		switch state {
		case webrtc.ICEConnectionStateFailed:
			if err := peerConnection.Close(); err != nil {
				panic(err)
			}
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
	localSDP = peerConnection.LocalDescription()
	// send RTP packets forever
	inboundRTPVPacket := make([]byte, 1600)
	inboundRTPAPacket := make([]byte, 1600)
	for running {
		Vn, _, err := Vlistener.ReadFrom(inboundRTPVPacket)
		if err != nil {
			panic(fmt.Sprintf("error during read: %s", err))
		}
		An, _, err := Alistener.ReadFrom(inboundRTPAPacket)
		if err != nil {
			panic(fmt.Sprintf("error during read: %s", err))
		}
		if _, err = videoTrack.Write(inboundRTPVPacket[:Vn]); err != nil {
			if errors.Is(err, io.ErrClosedPipe) {
				// connection closed
				return
			}
			panic(err)
		}
		if _, err = audioTrack.Write(inboundRTPAPacket[:An]); err != nil {
			if errors.Is(err, io.ErrClosedPipe) {
				// connection closed
				return
			}
			panic(err)
		}
	}
}

func main() {
	go serve(":8080")
	go rtcServer()
	fmt.Println("started")
	<-stopped
}
