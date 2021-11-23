'use strict';
window.onload = function() {
    console.log("loaded page");
    let localConnection = new RTCPeerConnection({
        iceServers: [
          {
            urls: 'stun:stun.l.google.com:19302'
          }
        ]
    });
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
    }
    document.getElementById("start").addEventListener("click", function() {
        console.log("Clicked button")
        // send offer
        let offer = localConnection.localDescription;
        axios.post("/sdp", {Type: offer.type, SDP: offer.sdp}).then(() => {
            console.log("Sent offer");
            // get answer
            axios.get("/sdp").then(response=>{
                let remoteDesc = new RTCSessionDescription(response.data);
                localConnection.setRemoteDescription(remoteDesc).then(() => {
                    console.log("Set Answer");
                });
            })
        })
    });

}
