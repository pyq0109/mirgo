package main

// 运行时窗口逻辑尺寸（== GLFW 窗口 CSS 像素），由 main.go 的 SizeCallback 更新。
// 初始值为设计尺寸；ScreenWidth/ScreenHeight 保留为底栏美术参考常量。
var winW, winH = ScreenWidth, ScreenHeight

// hudZoneH 底部 HUD 不透明区高度（底栏覆盖区域）。
const hudZoneH = ScreenHeight - MapSurfaceH // 155

// mapViewH 返回地图视口逻辑高度。
func mapViewH() int { return winH - hudZoneH }

// barOriginX 返回底栏（800px 宽）水平居中后的左偏移。
func barOriginX() int { return (winW - ScreenWidth) / 2 }
