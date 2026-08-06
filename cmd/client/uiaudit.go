package main

// UI 一致性审计 — 命中矩形 (Left/Top/Width/Height) 与绘制图素
// (WLib/FaceIndex + 图片尺寸/HotX/HotY) 是两套独立数据, 仅靠
// SetImgIndex 弱同步 (uicontrol.go), 大量手工覆盖导致 "按钮图与
// 点击范围不符"、"子控件错位" 类 bug。Audit 自动对照两套数据与
// 布局常识列出问题清单; 只报告不修复。控制台入口: ui audit。

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
)

type AuditLevel int

const (
	AuditError AuditLevel = iota
	AuditWarn
	AuditInfo
)

func (l AuditLevel) String() string {
	switch l {
	case AuditError:
		return "ERR"
	case AuditWarn:
		return "WARN"
	default:
		return "INFO"
	}
}

type AuditIssue struct {
	Path  string // DebugPath, 如 "DBackground>DItemBag>DItemGrid"
	Rule  string // 规则标识
	Level AuditLevel
	Msg   string
}

// auditWhitelist 已知有意偏离的白名单: key = "DebugPath|rule", value = 原因。
// 命中条目降级为 info 并附原因, 使 ui audit 收敛到真问题。
// 登记原则: 只登记 "故意的", 每个条目必须写清原因。
var auditWhitelist = map[string]string{}

// imageRect 返回带图控件的默认绘制矩形与图片偏移。
// BlitImage 语义 (uimanager.go): 画在 AbsX/AbsY, 取图片自身宽高,
// 不叠加 HotX/HotY; 而部分自定义 OnDirectPaint (如 uistate.go) 会
// 叠加 HotX/HotY — 两者同时返回供审计对照。
func (c *UIControl) imageRect() (x, y, w, h, hotX, hotY int, ok bool) {
	if c.WLib == nil {
		return 0, 0, 0, 0, 0, 0, false
	}
	img := c.WLib.GetImage(c.FaceIndex)
	if img == nil {
		return 0, 0, 0, 0, 0, 0, false
	}
	return c.AbsX(), c.AbsY(), int(img.Width), int(img.Height), int(img.HotX), int(img.HotY), true
}

// Audit 遍历整棵控件树 (含 Modal, 不论可见性) 收集问题, error 优先排序。
func (m *UIManager) Audit() []AuditIssue {
	var issues []AuditIssue
	m.auditWalk(m.Root, &issues)
	if m.Modal != nil {
		m.auditWalk(m.Modal, &issues)
	}
	sort.SliceStable(issues, func(i, j int) bool { return issues[i].Level < issues[j].Level })
	return issues
}

func (m *UIManager) auditWalk(c *UIControl, issues *[]AuditIssue) {
	m.auditControl(c, issues)
	for _, ch := range c.Children {
		m.auditWalk(ch, issues)
	}
}

func (m *UIManager) auditControl(c *UIControl, issues *[]AuditIssue) {
	add := func(rule string, level AuditLevel, format string, args ...interface{}) {
		path := c.DebugPath()
		msg := fmt.Sprintf(format, args...)
		if reason, ok := auditWhitelist[path+"|"+rule]; ok {
			level, msg = AuditInfo, msg+" [白名单: "+reason+"]"
		}
		*issues = append(*issues, AuditIssue{Path: path, Rule: rule, Level: level, Msg: msg})
	}

	w, h := c.effectiveSize()
	ax, ay := c.AbsX(), c.AbsY()

	// --- 图片相关: 命中矩形 vs 绘制图素 ---
	if c.WLib != nil {
		img := c.WLib.GetImage(c.FaceIndex)
		if img == nil {
			if c.OnDirectPaint == nil {
				add("img-missing", AuditError,
					"WLib[%d] 取图为 nil 且无自定义绘制: 有命中矩形但画不出任何东西", c.FaceIndex)
			}
		} else {
			iw, ih := int(img.Width), int(img.Height)
			if w != iw || h != ih {
				add("size-override", AuditWarn,
					"命中框 %dx%d ≠ 图片 %dx%d (SetImgIndex 后手工改过尺寸 — 图片与点击范围不符的主根源)",
					w, h, iw, ih)
			}
			if img.HotX != 0 || img.HotY != 0 {
				add("img-offset", AuditWarn,
					"图片偏移 HotX/HotY=(%d,%d) 非零: BlitImage/InRange 忽略偏移而自定义绘制叠加, 两者可能错位",
					img.HotX, img.HotY)
			}
		}
	}

	// --- 布局相关 ---
	if c.Parent != nil {
		pw, ph := c.Parent.effectiveSize()
		if c.Left < 0 || c.Top < 0 || c.Left+w > pw || c.Top+h > ph {
			add("outside-parent", AuditWarn,
				"矩形 [%d,%d %dx%d] 越出父控件 %s 的 [0,0 %dx%d]",
				c.Left, c.Top, w, h, c.Parent.Name, pw, ph)
		}
	}

	if ax+w <= 0 || ay+h <= 0 || ax >= ScreenWidth || ay >= ScreenHeight {
		add("offscreen", AuditWarn, "绝对矩形 [%d,%d %dx%d] 完全出屏 (%dx%d)", ax, ay, w, h, ScreenWidth, ScreenHeight)
	} else if ax < 0 || ay < 0 || ax+w > ScreenWidth || ay+h > ScreenHeight {
		add("offscreen", AuditInfo, "绝对矩形 [%d,%d %dx%d] 部分出屏", ax, ay, w, h)
	}

	// 同级可见按钮互相重叠 → 点击歧义 (树序靠后者赢)。
	for i := 0; i < len(c.Children); i++ {
		a := c.Children[i]
		if !a.Visible || a.Kind != KindButton || a.Background {
			continue
		}
		aw, ah := a.effectiveSize()
		for j := i + 1; j < len(c.Children); j++ {
			b := c.Children[j]
			if !b.Visible || b.Kind != KindButton || b.Background {
				continue
			}
			bw, bh := b.effectiveSize()
			if a.Left < b.Left+bw && b.Left < a.Left+aw && a.Top < b.Top+bh && b.Top < a.Top+ah {
				add("sibling-overlap", AuditWarn, "与兄弟按钮 %s 重叠 (命中歧义)", b.Name)
			}
		}
	}
}

// Validate 将审计结果写入日志 (error→Error, warn→Warn, info 不输出)。
// 各场景建树完成后调用一次; 不每帧运行。
func (m *UIManager) Validate() {
	issues := m.Audit()
	var nErr, nWarn, nInfo int
	for _, is := range issues {
		switch is.Level {
		case AuditError:
			nErr++
			log.Logf(log.LevelError, "UIAudit", "%s [%s] %s", is.Path, is.Rule, is.Msg)
		case AuditWarn:
			nWarn++
			log.Logf(log.LevelWarn, "UIAudit", "%s [%s] %s", is.Path, is.Rule, is.Msg)
		default:
			nInfo++
		}
	}
	if len(issues) == 0 {
		log.Logf(log.LevelDebug, "UIAudit", "validate: no issues")
		return
	}
	log.Logf(log.LevelInfo, "UIAudit", "validate: %d issues (%d error, %d warn, %d info)",
		len(issues), nErr, nWarn, nInfo)
}

// DebugAudit 返回人类可读的审计清单 (ui audit 命令)。
// filter 为空=全部, "err"=仅 error, 其他=路径子串过滤。
func (m *UIManager) DebugAudit(filter string) string {
	issues := m.Audit()
	if len(issues) == 0 {
		return "audit: no issues"
	}
	var nErr, nWarn, nInfo int
	for _, is := range issues {
		switch is.Level {
		case AuditError:
			nErr++
		case AuditWarn:
			nWarn++
		default:
			nInfo++
		}
	}
	var sb strings.Builder
	shown := 0
	for _, is := range issues {
		switch {
		case filter == "err" && is.Level != AuditError:
			continue
		case filter != "" && filter != "err" &&
			!strings.Contains(strings.ToLower(is.Path), strings.ToLower(filter)):
			continue
		}
		fmt.Fprintf(&sb, "%-4s %-15s %s\n     %s\n", is.Level, is.Rule, is.Path, is.Msg)
		shown++
	}
	if shown == 0 {
		fmt.Fprintf(&sb, "(no issue matches filter %q)\n", filter)
	}
	fmt.Fprintf(&sb, "--- total %d issues (%d error, %d warn, %d info), %d shown",
		len(issues), nErr, nWarn, nInfo, shown)
	return strings.TrimRight(sb.String(), "\n")
}
