package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// parseProtocolConsts 解析 internal/protocol/message.go 中指定前缀的常量名集合。
// 协议覆盖测试的事实源：常量增删必须同步更新路由/处理，否则测试失败。
func parseProtocolConsts(t *testing.T, prefix string) map[string]uint16 {
	t.Helper()
	fset := token.NewFileSet()
	path := filepath.Join("..", "..", "internal", "protocol", "message.go")
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	consts := make(map[string]uint16)
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
				continue
			}
			name := vs.Names[0].Name
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			bl, ok := vs.Values[0].(*ast.BasicLit)
			if !ok {
				continue
			}
			v, err := strconv.ParseUint(bl.Value, 0, 16)
			if err != nil {
				continue
			}
			consts[name] = uint16(v)
		}
	}
	if len(consts) == 0 {
		t.Fatalf("no %s constants parsed from %s", prefix, path)
	}
	return consts
}

// TestCMRoutingCoverage — 路线图 6.1 协议覆盖测试：
// 每个 CM 常量必须在 main.go 的路由 switch 中登记，或在豁免表中
// 显式注明原因。防止"处理器写了但路由没接上"类缺陷（P0-1）复发。
func TestCMRoutingCoverage(t *testing.T) {
	// 豁免表：已知不需要路由的 CM（必须附原因）。
	unimplemented := map[string]string{
		"CMThrow": "Delphi 全库无处理器（死消息，仅 Grobal2.pas:176 定义）→ backlog",
	}

	mainSrc, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	src := string(mainSrc)
	for name := range parseProtocolConsts(t, "CM") {
		if _, exempt := unimplemented[name]; exempt {
			continue
		}
		re := regexp.MustCompile(`protocol\.` + name + `\b`)
		if !re.MatchString(src) {
			t.Errorf("CM 常量 %s 未在 main.go 路由中登记：补 case 转发，或加入豁免表并注明原因", name)
		}
	}
	for name, reason := range unimplemented {
		if reason == "" {
			t.Errorf("豁免表 %s 的原因不能为空", name)
		}
	}
}

// TestAIRaceMappingCoverage — 路线图 6.1：怪物 race→AI 映射覆盖测试。
// 期望表来自 Delphi 怪物工厂（UsrEngn.pas:1819-1938）逐分支核对；
// 新增 race/AI 必须同步更新本表，防止 P0-2（race 134/214 映射遗漏）复发。
func TestAIRaceMappingCoverage(t *testing.T) {
	expected := map[byte]int{
		20:  AIGuard,          // TArcherPolice（TArcherGuard 子类）
		51:  AIPassive,        // 鸡
		52:  AIPassive,        // 鹿
		53:  AIMelee,          // 狼
		55:  AITrainer,        // 训练师沙袋
		80:  AIPassive,        // TMonster 基础游荡
		82:  AISpit,           // TSpitSpider
		83:  AIMelee,          // TSlowATMonster
		85:  AIBurrow,         // TStickMonster
		87:  AIDualAxe,        // TDualAxeMonster
		90:  AIPoison,         // TGasAttackMonster
		91:  AIMagicCast,      // TMagCowMonster
		92:  AICowKing,        // TCowKingMonster
		93:  AIRanged,         // TThornDarkMonster
		94:  AILightning,      // TLightingZombi
		95:  AIBurrow,         // TDigOutZombi
		96:  AISplit,          // TZilKinZombi
		100: AILevelingSkeleton, // TWhiteSkeleton
		101: AIStone,          // TScultureMonster
		102: AISummoner,       // TScultureKingMonster
		103: AISpawnHive,      // TBeeQueen
		104: AIRanged,         // TArcherMonster
		105: AIPoison,         // TGasMothMonster
		106: AIPoison,         // TGasDungMonster
		107: AICentiKing,      // TCentipedeKingMonster
		110: AIPassive,        // TCastleDoor
		111: AIPassive,        // TWallStructure
		112: AIGuard,          // TArcherGuard
		113: AITransform,      // TElfMonster
		114: AIBurrow,         // TElfWarriorMonster
		115: AIPulse,          // TBigHeartMonster
		116: AISpawnHive,      // TSpiderHouseMonster
		117: AIExplode,        // TExplosionSpider
		118: AISpit,           // THighRiskSpider
		119: AISpit,           // TBigPoisionSpider
		120: AISoccerBall,     // TSoccerBall
		130: AICritical,       // TDoubleCriticalMonster
		131: AIArea,           // TRonObject
		132: AIBurrow,         // TSandMobObject
		133: AIMagicCast,      // TMagicMonObject
		134: AIBoneKing,       // TBoneKingMonster
		200: AILeech,          // TElectronicScolpionMon
		201: AIClone,          // TClone
		203: AITeleport,       // TTeleMonster
		206: AIKhazard,        // TKhazard
		208: AIGreenPoison,    // TGreenMonster
		209: AIRedPoison,      // TRedMonster
		210: AIFrostTiger,     // TFrostTiger
		214: AIFireAura,       // TFireMonster
		215: AIFireball,       // TFireBallMonster
	}
	// 设计上走 getAIBehavior default 分支（近战）的 race：
	// Delphi TATMonster/TMonster 系（81/84/86/88/89/97）与人形怪（150/156）。
	meleeByDesign := map[byte]bool{
		81: true, 84: true, 86: true, 88: true, 89: true, 97: true,
		150: true, 156: true,
	}
	for race, want := range expected {
		if got := getAIBehavior(race); got != want {
			t.Errorf("race %d AI 映射 = %d，期望 %d（新增/调整 AI 请同步更新本测试）", race, got, want)
		}
	}
	for race := range meleeByDesign {
		if got := getAIBehavior(race); got != AIMelee {
			t.Errorf("race %d 应为默认 AIMelee，实际 %d", race, got)
		}
	}
	// 反向检查：任何非默认映射都必须登记在期望表中，
	// 防止新增 AI 映射绕过覆盖测试。
	for r := 0; r < 256; r++ {
		race := byte(r)
		got := getAIBehavior(race)
		if got == AIMelee {
			continue
		}
		if _, ok := expected[race]; !ok {
			t.Errorf("race %d 映射到非默认 AI %d，但未登记在期望表（请补充期望值与 Delphi 出处）", race, got)
		}
	}
}
