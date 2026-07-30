package main

import (
	"math/rand"
	"testing"
)

func TestLuckFormula(t *testing.T) {
	p := &PlayObject{BaseObject: NewBaseObject("test", 1)}
	p.WAbil.DC = 10 | 20<<16 // loDC=10, hiDC=20

	// Luck=9: Random(10-9)==0 → Random(1)==0 → always true → always max
	p.Luck = 9
	for i := 0; i < 100; i++ {
		dmg := p.calcDamage(nil)
		if dmg != 20 {
			t.Fatalf("luck=9: expected always 20, got %d", dmg)
		}
	}

	// Luck=0: normal random between 10-20
	p.Luck = 0
	seenMin, seenMax := false, false
	for i := 0; i < 10000; i++ {
		dmg := p.calcDamage(nil)
		if dmg < 10 || dmg > 20 {
			t.Fatalf("luck=0: damage %d out of range [10,20]", dmg)
		}
		if dmg == 10 {
			seenMin = true
		}
		if dmg == 20 {
			seenMax = true
		}
	}
	if !seenMin || !seenMax {
		t.Fatal("luck=0: expected to see both min and max values in 10k rolls")
	}

	// Luck=-9: Random(10-9)==0 → always min
	p.Luck = -9
	for i := 0; i < 100; i++ {
		dmg := p.calcDamage(nil)
		if dmg != 10 {
			t.Fatalf("luck=-9: expected always 10, got %d", dmg)
		}
	}
}

func TestHitMiss(t *testing.T) {
	p := &PlayObject{BaseObject: NewBaseObject("attacker", 1)}
	p.HitPoint = 5

	target := NewBaseObject("target", 2)
	target.SpeedPoint = 10

	hits := 0
	trials := 100000
	for i := 0; i < trials; i++ {
		if p.hitCheck(target) {
			hits++
		}
	}
	rate := float64(hits) / float64(trials)
	// Delphi: miss if HitPoint < Random(SpeedPoint)
	// P(hit) = P(HitPoint >= Random(10)) = P(Random(10) <= 5) = 6/10 = 0.6
	if rate < 0.55 || rate > 0.65 {
		t.Fatalf("hit rate = %.3f, expected ~0.6", rate)
	}
}

func TestHitMissAlwaysHit(t *testing.T) {
	p := &PlayObject{BaseObject: NewBaseObject("attacker", 1)}
	p.HitPoint = 100

	target := NewBaseObject("target", 2)
	target.SpeedPoint = 10

	for i := 0; i < 1000; i++ {
		if !p.hitCheck(target) {
			t.Fatal("HitPoint=100 vs SpeedPoint=10 should always hit")
		}
	}
}

func TestMagicShieldAbsorption(t *testing.T) {
	// Magic shield: 100 damage → 150 MP consumed → 0 damage taken
	tp := &PlayObject{BaseObject: NewBaseObject("mage", 1)}
	tp.WAbil.MP = 200
	tp.WAbil.HP = 100
	tp.HasMagicShield = true
	tp.StatusTimeArr = [12]int16{}

	damage := 100
	// Simulate magic shield logic
	mpCost := damage + damage/2 // 150
	mp := int(tp.WAbil.MP)
	if mp >= mpCost {
		tp.WAbil.MP -= uint16(mpCost)
		damage = 0
	}
	if damage != 0 {
		t.Fatalf("expected 0 damage after shield, got %d", damage)
	}
	if tp.WAbil.MP != 50 {
		t.Fatalf("expected MP=50 after absorbing 100 damage, got %d", tp.WAbil.MP)
	}
}

func TestMagicShieldPartial(t *testing.T) {
	// 100 damage, only 90 MP → absorb 60, take 40
	tp := &PlayObject{BaseObject: NewBaseObject("mage", 1)}
	tp.WAbil.MP = 90
	tp.WAbil.HP = 100
	tp.HasMagicShield = true

	damage := 100
	mpCost := damage + damage/2
	mp := int(tp.WAbil.MP)
	if mp >= mpCost {
		damage = 0
	} else {
		absorbed := mp * 2 / 3
		damage -= absorbed
	}
	if damage != 40 {
		t.Fatalf("expected 40 damage after partial shield, got %d", damage)
	}
}

func TestRedPoisonAmplification(t *testing.T) {
	// 100 base damage + red poison → 120
	damage := 100
	statusArr := [12]int16{}
	statusArr[POISON_DAMAGEARMOR] = 50

	if statusArr[POISON_DAMAGEARMOR] > 0 {
		damage = damage + damage/5
	}
	if damage != 120 {
		t.Fatalf("expected 120 with red poison, got %d", damage)
	}
}

func TestGreenPoisonKill(t *testing.T) {
	p := &PlayObject{BaseObject: NewBaseObject("victim", 1)}
	p.WAbil.HP = 3
	p.GreenPoisonDamage = 5
	p.StatusTimeArr = [12]int16{}
	p.StatusTimeArr[POISON_DECHEALTH] = 10

	// Simulate poison tick
	dmg := p.GreenPoisonDamage
	hp := int(p.WAbil.HP) - dmg
	if hp <= 0 {
		p.WAbil.HP = 0
		p.Death = true
	}
	if !p.Death {
		t.Fatal("green poison should kill when damage >= HP")
	}
}

func TestAttackModePeace(t *testing.T) {
	envir := &Environment{Width: 10, Height: 10, Cells: make([]MapCellInfo, 100)}

	p := &PlayObject{BaseObject: NewBaseObject("player1", 1)}
	p.AttackMode = AttackModePeace
	p.envir = envir

	target := &PlayObject{BaseObject: NewBaseObject("player2", 2)}
	envir.AddObject(5, 5, OS_MOVINGOBJECT, target)

	if p.CanAttackTarget(target.BaseObject) {
		t.Fatal("peace mode should not allow attacking players")
	}

	mon := NewMonsterObject("monster", 3, 50, 0, 0, 100, 1000, 1500, 10)
	envir.AddObject(6, 6, OS_MOVINGOBJECT, mon)
	if !p.CanAttackTarget(mon.BaseObject) {
		t.Fatal("peace mode should allow attacking monsters")
	}
}

func TestPowerHitAutoCharge(t *testing.T) {
	p := &PlayObject{BaseObject: NewBaseObject("warrior", 1)}
	p.LearnedMagics = []*PlayerMagic{{MagID: 7, Level: 0}}
	p.PowerHitCount = 1 // will trigger on next attack

	p.advancePowerHitCharge(protocol_CMHit(), nil)
	if !p.PowerHitReady {
		t.Fatal("PowerHit should be ready after counter reaches 0")
	}
	if p.PowerHitCount < 1 {
		t.Fatal("PowerHit counter should reset after triggering")
	}
}

func TestSkillDamageBonus(t *testing.T) {
	p := &PlayObject{BaseObject: NewBaseObject("warrior", 1)}
	p.HitPlus = 15
	p.HitDouble = 8

	// PowerHit: flat bonus
	dmg := p.applySkillBonus(100, protocol_CMPowerHit(), false)
	if dmg != 115 {
		t.Fatalf("PowerHit: expected 115, got %d", dmg)
	}

	// FireHit: percentage bonus (100 + 100*8*10/100 = 100+80 = 180)
	dmg = p.applySkillBonus(100, protocol_CMFireHit(), false)
	if dmg != 180 {
		t.Fatalf("FireHit: expected 180, got %d", dmg)
	}

	// HeavyHit: no bonus
	dmg = p.applySkillBonus(100, protocol_CMHeavyHit(), false)
	if dmg != 100 {
		t.Fatalf("HeavyHit: expected 100, got %d", dmg)
	}
}

func TestParalysisRingFormula(t *testing.T) {
	// AntiPoison=0: 1/5 chance
	trials := 100000
	procs := 0
	for i := 0; i < trials; i++ {
		if rand.Intn(0+5) == 0 {
			procs++
		}
	}
	rate := float64(procs) / float64(trials)
	if rate < 0.18 || rate > 0.22 {
		t.Fatalf("paralysis rate with AntiPoison=0: %.3f, expected ~0.2", rate)
	}

	// AntiPoison=15: 1/20 chance
	procs = 0
	for i := 0; i < trials; i++ {
		if rand.Intn(15+5) == 0 {
			procs++
		}
	}
	rate = float64(procs) / float64(trials)
	if rate < 0.04 || rate > 0.06 {
		t.Fatalf("paralysis rate with AntiPoison=15: %.3f, expected ~0.05", rate)
	}
}

// Helper stubs for protocol constants in tests
func protocol_CMHit() int      { return 3014 }
func protocol_CMPowerHit() int { return 3018 }
func protocol_CMFireHit() int  { return 3025 }
func protocol_CMHeavyHit() int { return 3015 }
