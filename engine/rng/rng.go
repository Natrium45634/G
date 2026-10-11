// Package rng — детерминированный генератор случайных чисел с сериализуемым состоянием.
//
// Используется SplitMix64: состояние — одно 64-битное число, поэтому его легко
// сохранять и восстанавливать. Генератор не зависит от платформы и FPS.
package rng

// RNG — генератор. Нулевое значение пригодно к использованию (seed = 0).
type RNG struct {
	State uint64 `json:"state"`
	Calls uint64 `json:"calls"`
}

// New создаёт генератор с заданным зерном.
func New(seed uint64) *RNG { return &RNG{State: seed} }

// Uint64 возвращает следующее псевдослучайное число.
func (r *RNG) Uint64() uint64 {
	r.Calls++
	r.State += 0x9E3779B97F4A7C15
	z := r.State
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// Intn возвращает число в [0, n). При n <= 0 возвращает 0.
func (r *RNG) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	// Отбрасывание для равномерности.
	limit := ^uint64(0) - (^uint64(0) % uint64(n))
	for {
		v := r.Uint64()
		if v < limit {
			return int(v % uint64(n))
		}
	}
}

// D100 — бросок 0..99. Проверка «успех при шансе p%» выглядит как D100() < p.
func (r *RNG) D100() int { return r.Intn(100) }

// Range возвращает число в [lo, hi] включительно.
func (r *RNG) Range(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + r.Intn(hi-lo+1)
}
