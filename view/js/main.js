'use strict';
let ws;
window.onload = function() {
    console.log("loaded page");
    let localConnection = new RTCPeerConnection({
        iceServers: [
          {
            urls: 'stun:stun.l.google.com:19302'
          }
        ]
    });
    // create websocket 
    ws = new WebSocket("ws://localhost:8080/ws");
    ws.onopen = function(evt) {
        console.log("OPEN");
    }
    ws.onclose = function(evt) {
        console.log("CLOSE");
        ws = null;
    }
    ws.onmessage = function(evt) {
        let responseJson = JSON.parse(evt.data);
        if (responseJson.status == 1) {
            if (responseJson.type == 0) {
                console.log("New message from ws: " + responseJson.response);
            }
            else if (responseJson.type == 1) {
                console.log("Got answer")
                let remoteDesc = new RTCSessionDescription(responseJson.response);
                localConnection.setRemoteDescription(remoteDesc).then(() => {
                    console.log("Set answer");
                });
            }
        }
        else {
            console.log("error in ws response: " + responseJson.response)
        }
        console.log(responseJson);
    }
    ws.onerror = function(evt) {
        console.log("ERROR: " + evt.data);
    }
    // create media stream
    const stream = new MediaStream();
    const remoteVideo = document.getElementById('stream');
    remoteVideo.srcObject = stream;
    localConnection.ontrack = (event) => {
        console.log(event.track.kind + " track received");
        stream.addTrack(event.track);
    }
    localConnection.oniceconnectionstatechange = e => console.log(localConnection.iceConnectionState);
    localConnection.addTransceiver('audio', {'direction': 'recvonly'})
    localConnection.addTransceiver('video', {'direction': 'recvonly'})

    localConnection.createOffer().then(offer => {
        localConnection.setLocalDescription(offer).then(()=>{
            console.log("Offer set");
            document.getElementById("start").disabled = false;

        }).catch(e=>{
            console.log("Error: "+e);
        });
    })
    // close connection on unload
    window.onbeforeunload = async function () {
        localConnection.close();
        localConnection = null;
        // TODO: send signal to close
    }
    document.getElementById("start").addEventListener("click", function() {
        console.log("Clicked button")
        // send offer
        let offer = localConnection.localDescription;
        ws.send(JSON.stringify({status: 1, type: 1, response: offer}))
        console.log("Sent offer");
    });

}
