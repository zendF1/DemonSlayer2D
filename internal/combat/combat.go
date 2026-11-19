// Package combat holds the pure combat formulas: character stats, damage and
// EXP. It has no state and no dependency beyond the standard library.
package combat

import (
	"math"
	"time"
)

// Default slash and level constants (spec 3.3, 3.4).
const (
	SlashRange     = 1.5                    // tiles, centre to centre
	SlashCooldown  = 800 * time.Millisecond // no MP cost
	MaxLevel       = 90
	PointsPerLevel = 10
)

// Points are the potential points a character has spent.
type Points struct{ Str, Agi, Vit, Int int }

// Total is the number of points spent.
func (p Points) Total() int { return p.Str + p.Agi + p.Vit + p.Int }

// Stats are the derived character stats.
type Stats struct {
	MaxHP, MaxMP              int
	PhysAttack, MagicAttack   int
	PhysDefense, MagicDefense int
	Accuracy, Evasion         int
	CritRateBP                int // basis points (1/10000)
}

// Compute derives stats from level and spent points (game-design 2.2, spec 3.1).
// magic selects the magic-class HP growth (a = 50, b = 16) instead of the
// physical one (a = 60, b = 20).
func Compute(magic bool, level int, p Points) Stats {
	l := level - 1
	hpA, hpB := 60, 20
	if magic {
		hpA, hpB = 50, 16
	}
	def := 10 + 2*l + 2*p.Vit
	return Stats{
		MaxHP:        500 + hpA*l + hpB*p.Vit,
		MaxMP:        100 + 20*l + 5*p.Int,
		PhysAttack:   20 + 5*l + 2*p.Str,
		MagicAttack:  20 + 5*l + 2*p.Int,
		PhysDefense:  def,
		MagicDefense: def,
		Accuracy:     100 + 5*l + 3*p.Agi,
		Evasion:      50 + 5*l + 2*p.Agi,
		CritRateBP:   500 + 2*p.Agi,
	}
}

// Damage is max(1, floor(base * (1 - def / (def + 20 * attackerLevel))))
// (spec 3.2). Hits always land and never crit.
func Damage(base, defense, attackerLevel int) int {
	d, k := float64(defense), 20*float64(attackerLevel)
	got := int(math.Floor(float64(base) * (1 - d/(d+k))))
	if got < 1 {
		return 1
	}
	return got
}

// SlashBase is the base damage of the default slash: the higher attack stat
// (spec 3.2, 3.3).
func SlashBase(s Stats) int { return max(s.PhysAttack, s.MagicAttack) }

// ExpToNext is the EXP needed to go from level to level+1 (game-design 2.2,
// spec 3.4): 10 * L * (10 + 2 * (L-1)^1.5) * multiplier, rounded to the
// nearest 10. The multiplier is 4L/3 below level 10 and 10 + 0.5 * (L-10)
// from level 10. Rounding is half-to-even to match docs/balance/exp_table.py
// (Python round). Returns 0 at MaxLevel.
func ExpToNext(level int) int64 {
	if level >= MaxLevel || level < 1 {
		return 0
	}
	l := float64(level)
	var mult float64
	if level < 10 {
		mult = 4 * l / 3
	} else {
		mult = 10 + 0.5*(l-10)
	}
	base := 10 * l * (10 + 2*math.Pow(l-1, 1.5))
	return int64(math.RoundToEven(base*mult/10)) * 10
}

// GainExp adds gain to the EXP of the current level. The remainder carries
// over, several levels can be gained at once, and the result stops at
// MaxLevel with exp 0. At MaxLevel nothing is gained.
func GainExp(level int, exp, gain int64) (newLevel int, newExp int64) {
	if level >= MaxLevel {
		return MaxLevel, 0
	}
	exp += gain
	for level < MaxLevel {
		need := ExpToNext(level)
		if exp < need {
			return level, exp
		}
		exp -= need
		level++
	}
	return MaxLevel, 0
}

// MonsterExp is a monster's total EXP, paid out over the hits on it by
// damage share (ExpShare): 10 * level (spec 3.4).
func MonsterExp(level int) int64 { return 10 * int64(level) }

// ExpShare is the part of a monster's total EXP that damage of its maxHP earns:
// floor(total * damage / maxHP), with damage capped at maxHP so a monster
// never pays more than total (spec 3.4). Integer math only. Paying the share
// of each attacker's summed damage, less what it was already paid, keeps the
// fractions that single hits would lose.
func ExpShare(total int64, maxHP, damage int) int64 {
	if total <= 0 || maxHP <= 0 || damage <= 0 {
		return 0
	}
	return total * int64(min(damage, maxHP)) / int64(maxHP)
}
