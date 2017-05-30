package main

import (
	"crypto"
	"log"

	"github.com/google/keytransparency/core/client/kt"
	"github.com/google/keytransparency/core/crypto/vrf"
	"github.com/google/keytransparency/core/crypto/vrf/p256"
	tpb "github.com/google/keytransparency/core/proto/keytransparency_v1_types"
	"github.com/google/keytransparency/core/tree/sparse"
	tv "github.com/google/keytransparency/core/tree/sparse/verifier"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/merkle/objhasher"
	"github.com/gopherjs/gopherjs/js"
	"golang.org/x/net/context"
)

const (
	sigPriv = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIHgSC8WzQK0bxSmfJWUeMP5GdndqUw8zS1dCHQ+3otj/oAoGCCqGSM49
AwEHoUQDQgAE5AV2WCmStBt4N2Dx+7BrycJFbxhWf5JqSoyp0uiL8LeNYyj5vgkl
K8pLcyDbRqch9Az8jXVAmcBAkvaSrLW8wQ==
-----END EC PRIVATE KEY-----`
	sigPub = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE5AV2WCmStBt4N2Dx+7BrycJFbxhW
f5JqSoyp0uiL8LeNYyj5vgklK8pLcyDbRqch9Az8jXVAmcBAkvaSrLW8wQ==
-----END PUBLIC KEY-----`
	priv = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIHgSC8WzQK0bxSmfJWUeMP5GdndqUw8zS1dCHQ+3otj/oAoGCCqGSM49
AwEHoUQDQgAE5AV2WCmStBt4N2Dx+7BrycJFbxhWf5JqSoyp0uiL8LeNYyj5vgkl
K8pLcyDbRqch9Az8jXVAmcBAkvaSrLW8wQ==
-----END EC PRIVATE KEY-----`
	pub = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE5AV2WCmStBt4N2Dx+7BrycJFbxhW
f5JqSoyp0uiL8LeNYyj5vgklK8pLcyDbRqch9Az8jXVAmcBAkvaSrLW8wQ==
-----END PUBLIC KEY-----`
)

func staticVRF() (vrf.PrivateKey, vrf.PublicKey, error) {
	vrf, err := p256.NewVRFSignerFromPEM([]byte(priv))
	if err != nil {
		return nil, nil, err
	}
	verfier, err := p256.NewVRFVerifierFromPEM([]byte(pub))
	if err != nil {
		return nil, nil, err
	}
	return vrf, verfier, nil
}

func staticKeyPair() (crypto.Signer, crypto.PublicKey, error) {
	sig, err := keys.NewFromPrivatePEM(sigPriv, "")
	if err != nil {
		return nil, nil, err
	}

	ver, err := keys.NewFromPublicPEM(sigPub)
	if err != nil {
		return nil, nil, err
	}
	return sig, ver, nil
}

func main() {
	js.Global.Call("alert", "Hello, JavaScript")
	_, vrfPub, err := staticVRF()
	if err != nil {
		log.Fatalf("Failed to load vrf keypair: %v", err)
	}
	_, verifier, err := staticKeyPair()
	if err != nil {
		log.Fatalf("Failed to load signing keypair: %v", err)
	}

	tlog := client.NewLogVerifier(objhasher.ObjectHasher, verifier)
	kt := kt.New(vrfPub, tv.New(0, sparse.CONIKSHasher), verifier, tlog)

	trusted := trillian.SignedLogRoot{}
	resp := &tpb.GetEntryResponse{}

	if err := kt.VerifyGetEntryResponse(context.Background(), "userid@example.com",
		"appFOO", &trusted, resp); err != nil {
		log.Fatalf("%v", err)
	}
	println("Hello, JS console")
}
