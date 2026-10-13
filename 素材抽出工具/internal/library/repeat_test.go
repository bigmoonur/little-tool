package library

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

func TestSparseSampleMatchesDenseFisherYates(t *testing.T) {
	pool := make([]string, 1000)
	for i := range pool {
		pool[i] = fmt.Sprint(i)
	}
	for n := 1; n < 20; n++ {
		for seed := uint64(0); seed < 20; seed++ {
			a, b := rand.New(rand.NewPCG(seed, seed)), rand.New(rand.NewPCG(seed, seed))
			got := sample(pool, n, a)
			idx := make([]int, len(pool))
			for i := range idx {
				idx[i] = i
			}
			for i := range n {
				j := i + b.IntN(len(idx)-i)
				idx[i], idx[j] = idx[j], idx[i]
				if got[i] != pool[idx[i]] {
					t.Fatalf("sparse sample diverged at %d: %q", i, got)
				}
			}
		}
	}
}

func TestRepeatCycleReusesSameOrder(t *testing.T) {
	l := newList("链接", "a", "a", "b", "c")
	for seed := uint64(0); seed < 20; seed++ {
		res, err := Extract([]List{l}, []PickRequest{{ListID: l.ID, Count: 2, Repeat: 4, Mode: RepeatCycle}}, rand.New(rand.NewPCG(seed, seed+1)))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(res.Text, "\n")
		if len(lines) != 8 || lines[0] == lines[1] {
			t.Fatalf("unexpected cycle: %q", lines)
		}
		for i := range lines {
			if lines[i] != lines[i%2] {
				t.Fatalf("cycle changed content or order: %q", lines)
			}
		}
	}
}

func TestRepeatRandomDrawsIndependentRounds(t *testing.T) {
	l := newList("链接", "a", "a", "b", "c", "d", "e")
	res, err := Extract([]List{l}, []PickRequest{{ListID: l.ID, Count: 2, Repeat: 4, Mode: RepeatRandom}}, testRand())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(res.Text, "\n")
	if len(lines) != 8 || res.Total != 8 {
		t.Fatalf("bad random total: %+v", res)
	}
	for i := 0; i < len(lines); i += 2 {
		if lines[i] == lines[i+1] {
			t.Fatalf("same round contains duplicate text: %q", lines)
		}
	}
	if countUnique(lines) <= 2 {
		t.Fatalf("random rounds only repeated the initial batch: %q", lines)
	}
	if _, err := Extract([]List{l}, []PickRequest{{ListID: l.ID, Count: 6, Repeat: 1, Mode: RepeatRandom}}, testRand()); err == nil {
		t.Fatal("random round must count distinct content")
	}
}

func TestRepeatModesRespectUnequalMultipliersAndLimits(t *testing.T) {
	a, b := newList("A", "a", "b"), newList("B", "c", "d")
	for _, mode := range []string{RepeatConsecutive, RepeatCycle, RepeatRandom} {
		res, err := Extract([]List{a, b}, []PickRequest{{ListID: a.ID, Count: 2, Repeat: 3, Mode: mode}, {ListID: b.ID, Count: 1, Repeat: 1, Mode: mode}}, testRand())
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(res.Text, "\n")
		if len(lines) != 7 {
			t.Fatalf("%s output length %d", mode, len(lines))
		}
		for _, text := range []string{"a", "b"} {
			count := 0
			for _, line := range lines {
				if line == text {
					count++
				}
			}
			if count != 3 {
				t.Fatalf("%s lost a repeat: %q", mode, lines)
			}
		}
		if len(res.Parts) != 2 || res.Total != 7 || res.Parts[0].Mode != mode {
			t.Fatalf("bad metadata: %+v", res)
		}
		if _, err := Extract([]List{a}, []PickRequest{{ListID: a.ID, Count: 2, Repeat: int(^uint(0) >> 1), Mode: mode}}, testRand()); err == nil {
			t.Fatal("overflow multiplier accepted")
		}
	}
	if _, err := Extract([]List{a, b}, []PickRequest{{ListID: a.ID, Count: 1, Repeat: 2, Mode: RepeatCycle}, {ListID: b.ID, Count: 1, Repeat: 2, Mode: RepeatRandom}}, testRand()); err == nil {
		t.Fatal("mixed global rules accepted")
	}
}
