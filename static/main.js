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
        console.log("here");
    }
    document.getElementById("start").addEventListener("click", function() {
        console.log("Clicked button")
        // send offer
        let postXhr = new XMLHttpRequest();
        postXhr.open("POST", "/sdp", true);
        postXhr.setRequestHeader("Content-type", "application/json");
        let offer = localConnection.localDescription;
        postXhr.send(JSON.stringify({Type: offer.type, SDP: offer.sdp}));
        postXhr.onload = function () {
            console.log("Sent offer");
            // get answer
            let getXhr = new XMLHttpRequest();
            getXhr.open("GET", "/sdp", true);
            getXhr.onload = function(){
                let remoteDesc = new RTCSessionDescription(JSON.parse(this.response));
                localConnection.setRemoteDescription(remoteDesc).then(() => {
                    console.log("Set Answer");
                });
            }
            getXhr.send();
        }
    });

}
