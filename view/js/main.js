'use strict';
const localConnection = new RTCPeerConnection({
    iceServers: [
      {
        urls: 'stun:stun.l.google.com:19302'
      }
    ]
});
function createRTCConnection(ws) {
    // on ice candidate
    localConnection.onicecandidate = function(event) {
        if (event.candidate == null) {
            return;
        }
        ws.send(JSON.stringify({status: 1, type: 2, response: event.candidate}));
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
    localConnection.addTransceiver('audio', {'direction': 'recvonly'});
    localConnection.addTransceiver('video', {'direction': 'recvonly'});

    localConnection.createOffer().then(offer => {
        localConnection.setLocalDescription(offer).then(()=>{
            console.log("Offer set");
            document.getElementById("start").disabled = false;

        }).catch(e=>{
            console.log("Error: "+e);
        });
    })
    document.getElementById("start").addEventListener("click", function() {
        console.log("Clicked button")
        // send offer
        let offer = localConnection.localDescription;
        ws.send(JSON.stringify({status: 1, type: 1, response: offer}));
        console.log("Sent offer");
    });
}
window.onload = function() {
    console.log("loaded page");
    
    // create websocket connection 
    let ws = new WebSocket("ws://localhost:8080/ws?token=ey.wirkstaufmichsowieichaufdich");
    ws.onopen = function(evt) {
        console.log("OPEN");
        createRTCConnection(ws);
    }
    ws.onclose = function(evt) {
        console.log("CLOSE");
        ws = null;
    }
    ws.onmessage = function(evt) {
        let responseJson = JSON.parse(evt.data);
        console.log(responseJson);
        if (responseJson.status == 1) {
            if (responseJson.type == 0) { // string message
                console.log("New message from ws: " + responseJson.response);
            }
            else if (responseJson.type == 1) { // sdp message
                console.log("Got answer")
                let remoteDesc = new RTCSessionDescription(responseJson.response);
                localConnection.setRemoteDescription(remoteDesc).then(() => {
                    console.log("Set answer");
                });
            }
            else if (responseJson.type == 2) { // ice candidate message
                localConnection.addIceCandidate(new RTCIceCandidate(responseJson.response)).then(()=>{
                    console.log("Added new Ice candidate:");
                    console.log(responseJson.response);
                });
                // TODO: add this
                // TODO: research what are ice candidates doing exactly
                // TODO: fix bugs on page refresh
                // TODO: document why on ice candidate is required
            }
        }
        else {
            console.log("error in ws response: " + responseJson.response)
        }
    }
    ws.onerror = function(evt) {
        console.log("ERROR: " + evt.data);
    }
    
    // close connection on unload
    window.onbeforeunload = async function () {
        localConnection.close();
        localConnection = null;
        // TODO: send signal to close
    }

}
