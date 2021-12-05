// functions for http server
package main

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

func serve_backend(port string) {
	router := gin.Default()
	router.Static("/static", "./view")
	router.LoadHTMLGlob("view/html/*")
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})
	router.GET("/ws", func(c *gin.Context) {
		room_code := c.Request.URL.Query().Get("token")
		if auth_room_code(room_code) {
			// communication channel between websocket and rtc
			SockToRTC := make(chan Message, 10)
			RTCToSock := make(chan Message, 10)
			// write to this channel after each function is ended
			// this way be more confident no dangling threads exist
			ended := make(chan bool, 2)
			// set response status
			// start socket and rtc handlers
			c.Status(http.StatusOK)
			go start_websocket(&c, RTCToSock, SockToRTC, ended)
			go start_rtc(SockToRTC, RTCToSock, ended)
			// wait for both to end
			<-ended
			<-ended
			fmt.Println("Both Ended")
		} else {
			c.Status(http.StatusUnauthorized)
		}
	})
	router.Run(port)
}
