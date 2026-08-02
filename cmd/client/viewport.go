package main

// winW, winH 跟踪实际窗口像素尺寸，仅用于鼠标坐标缩放。
// 逻辑坐标固定 800×600（ScreenWidth/ScreenHeight）。
var winW, winH = ScreenWidth, ScreenHeight
