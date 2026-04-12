package model

import _ "embed"

//go:embed hooks/bash.bash
var EmbeddedBashHook []byte

//go:embed hooks/zsh.zsh
var EmbeddedZshHook []byte

//go:embed hooks/fish.fish
var EmbeddedFishHook []byte
