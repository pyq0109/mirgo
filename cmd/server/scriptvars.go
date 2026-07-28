package main

import "sync"

type GlobalScriptVars struct {
	mu sync.RWMutex
	G  [20]int  // 全局变量 G0-G19
	I  [100]int // 全局动态变量 I0-I99
}

var globalScriptVars GlobalScriptVars
