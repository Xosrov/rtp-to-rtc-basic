// +build !js
// before running, execute this ffmpeg screen grabber:
// ffmpeg -re -f x11grab -draw_mouse 0 -show_region 1 -grab_x 0 -grab_y 185 -video_size 1920x870 -i :0 -c:v libvpx  -f rtp 'rtp://127.0.0.1:5004?pkt_size=1200'
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pion/webrtc/v3"
)

var stopped chan bool = make(chan bool, 1)
var localSDP *webrtc.SessionDescription = nil
var remoteSDP chan webrtc.SessionDescription = make(chan webrtc.SessionDescription, 1)

func serve(port string) {
	// send stop signal if exit
	defer func() {
		stopped <- true
	}()
	// start the front-end serving and receiving
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, r.URL.Path[1:])
	})
	http.HandleFunc("/sdp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			decoder := json.NewDecoder(r.Body)
			answer := webrtc.SessionDescription{}
			decoder.Decode(&answer)
			select {
			case remoteSDP <- answer:
			default:
				fmt.Println("Channel is still not emptied, disregarding new request")
			}
			fmt.Println("Got client sdp")
		} else if r.Method == "GET" {
			// wait for local sdp to be ready
			for localSDP == nil {
				time.Sleep(time.Second / 2)
			}
			w.Header().Add("Content-Type", "application/json")
			data, err := json.Marshal(*localSDP)
			if err != nil {
				panic(err)
			}
			w.Write(data)
			fmt.Println("Told client sdp")
			// empty for next client, not the best way but works
			localSDP = nil
		}

	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
func new_serve(port string) {
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
	// UDP listener for RTP on port 5004
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5004})
	if err != nil {
		panic(err)
	}
	// make sure listener is closed after done
	defer func() {
		if err = listener.Close(); err != nil {
			panic(err)
		}
	}()

	// video track
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "testing")
	if err != nil {
		panic(err)
	}
	// add track to lc

	rtpSender, err := peerConnection.AddTrack(videoTrack)
	if err != nil {
		panic(err)
	}
	// keep reading from track unless error occurs
	go func() {
		rtcpBuffer := make([]byte, 1500)
		for {
			if _, _, err := rtpSender.Read(rtcpBuffer); err != nil {
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
	inboundRTPPacket := make([]byte, 1600)
	for running {
		n, _, err := listener.ReadFrom(inboundRTPPacket)
		if err != nil {
			panic(fmt.Sprintf("error during read: %s", err))
		}

		if _, err = videoTrack.Write(inboundRTPPacket[:n]); err != nil {
			if errors.Is(err, io.ErrClosedPipe) {
				// connection closed
				return
			}

			panic(err)
		}
	}
}

func main() {
	go new_serve(":8080")
	go rtcServer()
	fmt.Println("started")
	<-stopped
}
