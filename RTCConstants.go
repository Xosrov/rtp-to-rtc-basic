// Define constants for RTC server
package main

import (
	"errors"
	"io"
	"net"

	"github.com/pion/webrtc/v3"
)

// video/audio tracks
var videoTrack, audioTrack *webrtc.TrackLocalStaticRTP

// video/audio listeners, make sure to close these after main is completed
var Vlistener, Alistener *net.UDPConn

// video/audio ports
var Vport = 5004
var Aport = 5005

// audio/video formats
var VCodec = webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}
var ACodec = webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}

var RTCBufferSize = 1500

func init() {
	var err error
	Vlistener, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: Vport})
	if err != nil {
		panic(err)
	}
	Alistener, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: Aport})
	if err != nil {
		panic(err)
	}
	// video track
	videoTrack, err = webrtc.NewTrackLocalStaticRTP(VCodec, "video", "vid")
	if err != nil {
		panic(err)
	}
	// audio track
	audioTrack, err = webrtc.NewTrackLocalStaticRTP(ACodec, "audio", "aud")
	if err != nil {
		panic(err)
	}
	// start video track reader(reads while listeners are open, make sure to close them)
	go func() {
		inboundRTPPacket := make([]byte, 1600)
		for {
			n, _, err := Vlistener.ReadFrom(inboundRTPPacket)
			if err != nil {
				return
			}
			if _, err = videoTrack.Write(inboundRTPPacket[:n]); err != nil {
				if errors.Is(err, io.ErrClosedPipe) {
					return
				}
				panic(err)
			}
		}
	}()
	// start audio track reader(reads while listeners are open, make sure to close them)
	go func() {
		inboundRTPPacket := make([]byte, 1600)
		for {
			n, _, err := Alistener.ReadFrom(inboundRTPPacket)
			if err != nil {
				return
			}
			if _, err = audioTrack.Write(inboundRTPPacket[:n]); err != nil {
				if errors.Is(err, io.ErrClosedPipe) {
					return
				}
				panic(err)
			}
		}
	}()
}
