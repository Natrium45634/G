package rng

import "testing"

func TestDeterministicAndRestorable(t *testing.T) {
	a, b := New(42), New(42)
	for i := 0; i < 1000; i++ {
		if a.Uint64() != b.Uint64() {
			t.Fatal("одинаковое зерно дало разные последовательности")
		}
	}
	saved := *a
	x := a.Intn(1000)
	restored := saved
	if restored.Intn(1000) != x {
		t.Fatal("восстановленное состояние даёт другой результат")
	}
}

func TestIntnBoundsAndDistribution(t *testing.T) {
	r := New(7)
	counts := make([]int, 10)
	for i := 0; i < 100000; i++ {
		v := r.Intn(10)
		if v < 0 || v >= 10 {
			t.Fatalf("значение вне диапазона: %d", v)
		}
		counts[v]++
	}
	for i, c := range counts {
		if c < 9000 || c > 11000 {
			t.Fatalf("неравномерное распределение: %d → %d", i, c)
		}
	}
	if r.Intn(0) != 0 || r.Range(5, 5) != 5 {
		t.Fatal("граничные случаи")
	}
}
