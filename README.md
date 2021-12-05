# rtp-to-rtc-basic  

## Usage  
1. Run ffmpeg command for video(VP8) on port 5004, for example: `ffmpeg -re -f lavfi -i testsrc=size=640x480:rate=30 -vcodec libvpx -cpu-used 5 -deadline 1 -g 10 -error-resilient 1 -auto-alt-ref 1 -f rtp 'rtp://127.0.0.1:5004?pkt_size=1200'`
2. Run ffmpeg command for audio(Opus) on port 5005, for example: `ffmpeg -f lavfi -i 'sine=frequency=1000' -c:a libopus -b:a 48000 -sample_fmt s16p -ssrc 1 -payload_type 111 -f rtp -max_delay 0 -application lowdelay 'rtp://127.0.0.1:5005?pkt_size=1200'`
3. Run all go files, for example `go run *.go`
4. Visit `localhost:8080`