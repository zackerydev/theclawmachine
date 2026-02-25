package main

import _ "embed"

var (
	//go:embed assets/kind/default.yaml
	kindConfigDefaultCNI []byte
	//go:embed assets/kind/cilium.yaml
	kindConfigCilium []byte
)
