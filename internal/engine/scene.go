package engine

import "github.com/pyq0109/mirgo/internal/log"

// SceneType 表示游戏场景的类型。
// 对应 Delphi IntroScn.pas 中的 TSceneType 枚举
type SceneType int

const (
	SceneIntro         SceneType = iota // 0：启动画面（空）
	SceneLogin                          // 1：账号/密码登录界面
	SceneSelectServer                   // 2：选服对话框
	SceneSelectChr                      // 3：角色选择
	SceneNewChr                         // 4：创建角色（未使用）
	SceneLoading                        // 5：加载界面（未使用）
	SceneLoginNotice                    // 6：登录公告
	ScenePlayGame                       // 7：主游戏场景
)

// String 返回场景类型名称。
func (t SceneType) String() string {
	switch t {
	case SceneIntro:
		return "Intro"
	case SceneLogin:
		return "Login"
	case SceneSelectServer:
		return "SelectServer"
	case SceneSelectChr:
		return "SelectChr"
	case SceneNewChr:
		return "NewChr"
	case SceneLoading:
		return "Loading"
	case SceneLoginNotice:
		return "LoginNotice"
	case ScenePlayGame:
		return "PlayGame"
	default:
		return "Unknown"
	}
}

// Scene 是所有游戏场景的接口。
type Scene interface {
	// Open 在场景变为活动时调用。
	Open()
	// Close 在场景变为非活动时调用。
	Close()
	// Update 更新场景状态。
	Update(dt float64)
	// Render 渲染场景。
	Render(gl *GLState, proj [16]float32)
	// OnKey 处理键盘输入。
	OnKey(key int, action int)
	// OnMouse 处理鼠标按键输入。mods 是 GLFW 修饰键的位掩码。
	OnMouse(x, y float64, button int, action int, mods int)
	// OnScroll 处理鼠标滚轮输入。
	OnScroll(x, y float64)
}

// SceneManager 管理场景切换。
type SceneManager struct {
	currentType SceneType
	current     Scene
	scenes      map[SceneType]Scene
}

// NewSceneManager 创建一个新的场景管理器。
func NewSceneManager() *SceneManager {
	return &SceneManager{
		scenes: make(map[SceneType]Scene),
	}
}

// RegisterScene 为指定类型注册一个场景。
func (m *SceneManager) RegisterScene(t SceneType, scene Scene) {
	m.scenes[t] = scene
}

// ChangeScene 切换到一个新场景。
func (m *SceneManager) ChangeScene(t SceneType) {
	log.Logf(log.LevelInfo, "Scene", "切换场景：%s → %s", m.currentType, t)
	if m.current != nil {
		m.current.Close()
	}
	m.currentType = t
	m.current = m.scenes[t]
	if m.current != nil {
		m.current.Open()
	}
}

// CurrentType 返回当前场景类型。
func (m *SceneManager) CurrentType() SceneType {
	return m.currentType
}

// Current 返回当前场景。
func (m *SceneManager) Current() Scene {
	return m.current
}

// Update 更新当前场景。
func (m *SceneManager) Update(dt float64) {
	if m.current != nil {
		m.current.Update(dt)
	}
}

// Render 渲染当前场景。
func (m *SceneManager) Render(gl *GLState, proj [16]float32) {
	if m.current != nil {
		m.current.Render(gl, proj)
	}
}

// OnKey 将键盘输入转发给当前场景。
func (m *SceneManager) OnKey(key int, action int) {
	if m.current != nil {
		m.current.OnKey(key, action)
	}
}

// OnMouse 将鼠标按键输入转发给当前场景。
func (m *SceneManager) OnMouse(x, y float64, button int, action int, mods int) {
	if m.current != nil {
		m.current.OnMouse(x, y, button, action, mods)
	}
}

// OnScroll 将鼠标滚轮输入转发给当前场景。
func (m *SceneManager) OnScroll(x, y float64) {
	if m.current != nil {
		m.current.OnScroll(x, y)
	}
}

// OnChar 在场景支持时将字符输入转发给当前场景。
func (m *SceneManager) OnChar(char rune) {
	if s, ok := m.current.(interface{ OnChar(rune) }); ok {
		s.OnChar(char)
	}
}

// OnMouseMove 在场景支持时将光标移动转发给当前场景。
func (m *SceneManager) OnMouseMove(x, y float64) {
	if s, ok := m.current.(interface{ OnMouseMove(x, y float64) }); ok {
		s.OnMouseMove(x, y)
	}
}
