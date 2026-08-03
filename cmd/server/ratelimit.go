package main

import (
	"sync"
	"time"
)

// 本文件移植 Delphi 服务端针对登录期消息的限流与状态标记：
//   - 注册/改密的 5 秒连接级限流（LoginSrv/LMain.pas:979-996）
//   - 查角/建角/删角共享的 dwChrTick 限流（DBServer/UsrSoc.pas:536-575）
//   - boChrQueryed「选角前须先查角」状态（UsrSoc.pas:535-590）

type throttleKey struct {
	sessionID int64
	tag       string
}

var msgThrottles = struct {
	sync.Mutex
	last map[throttleKey]time.Time
}{last: make(map[throttleKey]time.Time)}

// throttleOK 返回该会话该类消息距上次是否已过 interval。允许时记录本次时间。
func throttleOK(sessionID int64, tag string, interval time.Duration) bool {
	msgThrottles.Lock()
	defer msgThrottles.Unlock()
	k := throttleKey{sessionID, tag}
	now := time.Now()
	if t, ok := msgThrottles.last[k]; ok && now.Sub(t) <= interval {
		return false
	}
	msgThrottles.last[k] = now
	return true
}

// chrTicks 对应 Delphi 的 dwChrTick：QUERYCHR/NEWCHR/DELCHR 共享同一时间戳，
// 阈值按消息区分（查询 200ms，建角/删角 1000ms，UsrSoc.pas:536,547,562）。
var chrTicks = struct {
	sync.Mutex
	last map[int64]time.Time
}{last: make(map[int64]time.Time)}

// chrThrottleOK 检查并刷新会话的 chr 操作时间戳。
func chrThrottleOK(sessionID int64, minInterval time.Duration) bool {
	chrTicks.Lock()
	defer chrTicks.Unlock()
	now := time.Now()
	if t, ok := chrTicks.last[sessionID]; ok && now.Sub(t) <= minInterval {
		return false
	}
	chrTicks.last[sessionID] = now
	return true
}

// chrQueryStates 对应 Delphi 的 boChrQueryed（UsrSoc.pas）。
// 查角成功置 true；建角/删角/选角成功置 false。
var chrQueryStates = struct {
	sync.Mutex
	queried map[int64]bool
}{queried: make(map[int64]bool)}

func setChrQueried(sessionID int64, v bool) {
	chrQueryStates.Lock()
	if v {
		chrQueryStates.queried[sessionID] = true
	} else {
		delete(chrQueryStates.queried, sessionID)
	}
	chrQueryStates.Unlock()
}

func getChrQueried(sessionID int64) bool {
	chrQueryStates.Lock()
	defer chrQueryStates.Unlock()
	return chrQueryStates.queried[sessionID]
}

// clearSessionThrottles 在会话断开时清理其限流与查角状态。
func clearSessionThrottles(sessionID int64) {
	msgThrottles.Lock()
	for k := range msgThrottles.last {
		if k.sessionID == sessionID {
			delete(msgThrottles.last, k)
		}
	}
	msgThrottles.Unlock()

	chrTicks.Lock()
	delete(chrTicks.last, sessionID)
	chrTicks.Unlock()

	setChrQueried(sessionID, false)
}
