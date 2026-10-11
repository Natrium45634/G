package combat

import (
	"testing"

	"hextactics/engine/data"
	"hextactics/engine/hex"
	"hextactics/engine/rng"
)

func rules(t *testing.T) *data.Rules {
	c, err := data.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	return &c.Rules
}

// Сценарий C: атаки с разных направлений дают правильный сектор — для всех шести ориентаций.
func TestSectorsForAllSixFacings(t *testing.T) {
	def := hex.Hex{}
	for f := hex.Dir(0); f < 6; f++ {
		for a := hex.Dir(0); a < 6; a++ {
			attacker := def.Neighbor(a)
			s, dir, ok := SectorOf(def, f, attacker)
			if !ok || dir != a {
				t.Fatalf("не определено направление угрозы f=%d a=%d", f, a)
			}
			want := Sector(hex.Delta(f, a))
			if s != want {
				t.Fatalf("взгляд %d, атака с %d: сектор %d, ожидался %d", f, a, s, want)
			}
		}
		// Спереди — фронт, сзади — тыл, на дальней дистанции тоже
		if s, _, _ := SectorOf(def, f, f.Vec().Scale(4)); s != Front {
			t.Fatalf("дальняя атака спереди классифицирована как %v", s)
		}
		if s, _, _ := SectorOf(def, f, f.Opposite().Vec().Scale(3)); s != Rear {
			t.Fatalf("дальняя атака сзади классифицирована как %v", s)
		}
	}
	counts := map[Sector]int{}
	for a := hex.Dir(0); a < 6; a++ {
		s, _, _ := SectorOf(def, 0, def.Neighbor(a))
		counts[s]++
	}
	if counts[Front] != 1 || counts[NearFlank] != 2 || counts[FarFlank] != 2 || counts[Rear] != 1 {
		t.Fatalf("распределение секторов вокруг юнита неверно: %v", counts)
	}
}

func TestSectorTieFavoursDefender(t *testing.T) {
	// Угроза из (1,1): ровно между В(0) и ЮВ(5). Смотрит на В → фронт, а не ближний фланг.
	if s, _, _ := SectorOf(hex.Hex{}, 0, hex.Hex{Q: 1, R: 1}); s != Front {
		t.Fatalf("ожидался фронт, получено %v", s)
	}
	if s, _, _ := SectorOf(hex.Hex{}, 2, hex.Hex{Q: 1, R: 1}); s != FarFlank {
		t.Fatalf("смотрит на СЗ: ожидался дальний фланг (Δ=2 вместо 3), получено %v", s)
	}
}

func baseInput(sector Sector) *AttackInput {
	return &AttackInput{
		Kind: "melee", Sector: sector, Accuracy: 80, Evadable: true, Evasion: 20, Blockable: true,
		Block: 50, BlockPower: 60, CritChance: 10, CritDamage: 150,
		Damage: []DamageInput{{Type: "physical", TypeName: "физ", Raw: 20, RawText: "20", ArmorApplies: true, Armor: 5}},
	}
}

func TestSectorModifiersApplied(t *testing.T) {
	r := rules(t)
	for s := Front; s <= Rear; s++ {
		p := Estimate(baseInput(s), r)
		if p.SurpriseChance != r.Sectors.Surprise[s] {
			t.Fatalf("сектор %v: неожиданность %d", s, p.SurpriseChance)
		}
		if p.EvadeChance != 20*r.Sectors.Evasion[s]/100 {
			t.Fatalf("сектор %v: уклонение %d", s, p.EvadeChance)
		}
		if p.BlockChance != 50*r.Sectors.FrontBlock[s]/100 {
			t.Fatalf("сектор %v: блок %d", s, p.BlockChance)
		}
	}
	if Estimate(baseInput(Rear), r).BlockChance != 0 {
		t.Fatal("блок со спины должен быть 0")
	}
}

// Попадание, уклонение и неожиданность — независимые броски.
func TestRollsAreIndependent(t *testing.T) {
	r := rules(t)
	g := rng.New(1)
	in := baseInput(NearFlank) // неожиданность 30%
	in.Block = 0
	n := 40000
	var surprised, hit, surprisedAndHit int
	for i := 0; i < n; i++ {
		o := Resolve(in, r, g)
		if o.Surprised {
			surprised++
		}
		if o.Hit {
			hit++
		}
		if o.Surprised && o.Hit {
			surprisedAndHit++
		}
	}
	ps, ph, psh := float64(surprised)/float64(n), float64(hit)/float64(n), float64(surprisedAndHit)/float64(n)
	if ps < 0.28 || ps > 0.32 || ph < 0.78 || ph > 0.82 {
		t.Fatalf("частоты не соответствуют шансам: неожиданность %.3f, попадание %.3f", ps, ph)
	}
	if d := psh - ps*ph; d > 0.01 || d < -0.01 {
		t.Fatalf("броски зависимы: P(S∧H)=%.3f, P(S)P(H)=%.3f", psh, ps*ph)
	}
	// 100% неожиданности не означает автопопадание или автокрит
	in2 := baseInput(Rear)
	in2.Accuracy = 0 // → hit_min
	misses := 0
	for i := 0; i < 2000; i++ {
		o := Resolve(in2, r, g)
		if !o.Surprised {
			t.Fatal("атака в тыл без бонусов должна быть неожиданной")
		}
		if !o.Hit {
			misses++
		}
		if o.Crit && o.CritChance >= 100 {
			t.Fatal("неожиданность дала автокрит")
		}
	}
	if misses < 1700 {
		t.Fatalf("неожиданная атака не должна гарантировать попадание: промахов %d из 2000", misses)
	}
}

func TestSurpriseNegatesActiveDefenseAndExplains(t *testing.T) {
	r := rules(t)
	in := baseInput(Front)
	in.ForceSurprise = "цель оглушена"
	in.Accuracy = 200
	o := Resolve(in, r, rng.New(3))
	if !o.Surprised || o.Evaded || o.Blocked || !o.Hit {
		t.Fatalf("при неожиданности уклонение и блок не применяются: %+v", o)
	}
	if len(o.Lines) < 5 {
		t.Fatalf("недостаточно подробное объяснение: %v", o.Lines)
	}
}

func TestMitigationOrder(t *testing.T) {
	r := rules(t)
	d := DamageInput{Type: "physical", TypeName: "физ", Raw: 20, RawText: "20", ArmorApplies: true, Armor: 10, Resist: 50, Vuln: 20}
	// 20 → крит 150% = 30 → броня 10×40% = −4 → 26 → сопр 50% = 13 → уязв +20% = 15.6 → блок −50% = 7.8 → 8
	amt, lines := Mitigate(d, MitigationOpts{Crit: true, CritDamage: 150, PositionalPct: 40, Blocked: true, BlockPower: 50, Rules: r}, rng.New(1))
	if amt != 8 {
		t.Fatalf("ожидалось 8, получено %d (%v)", amt, lines)
	}
	d.Immune = true
	if amt, _ := Mitigate(d, MitigationOpts{Rules: r}, rng.New(1)); amt != 0 {
		t.Fatal("иммунитет не обнулил урон")
	}
	weak := DamageInput{Type: "physical", Raw: 2, RawText: "2", ArmorApplies: true, Armor: 50}
	if amt, _ := Mitigate(weak, MitigationOpts{PositionalPct: 100, Rules: r}, rng.New(1)); amt != r.MinDamage {
		t.Fatalf("минимальный урон: %d", amt)
	}
}
