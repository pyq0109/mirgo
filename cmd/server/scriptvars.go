package main

import "sync"

type GlobalScriptVars struct {
	mu      sync.RWMutex
	G       [20]int
	I       [100]int
	StrVars map[string]string
}

var globalScriptVars = GlobalScriptVars{
	StrVars: make(map[string]string),
}
