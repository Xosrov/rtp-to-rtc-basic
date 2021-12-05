// an example of screen graber to use in conjunction:
// ffmpeg -re -f x11grab -draw_mouse 0 -show_region 1 -grab_x 0 -grab_y 185 -video_size 1920x870 -i :0 -c:v libvpx  -f rtp 'rtp://127.0.0.1:5004?pkt_size=1200'
package main

func main() {
	// cleanups
	defer func() {
		// make sure listeners are closed
		if err := Alistener.Close(); err != nil {
			panic(err)
		}
		if err := Vlistener.Close(); err != nil {
			panic(err)
		}
	}()
	serve_backend(":8080")
}
