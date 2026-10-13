package main

import (
	"cmp"
	"math/rand"
	"slices"
	"sort"
	"testing"
)

// The sprite sorts give the same order as the sort.Slice / sort.SliceStable code they replace
func TestSpriteSortOrders(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 200; round++ {
		n := rng.Intn(40) + 1
		dl := make(DrawList, n)
		for i := range dl {
			dl[i] = &SpriteData{
				priority: int32(rng.Intn(4)), syncGroup: rng.Intn(3), syncId: int32(rng.Intn(3)),
				syncLayer: int32(rng.Intn(3)), sortindex: i,
			}
		}
		// The draw list, as sorted before
		oldList := make([]int, n)
		for i := range oldList {
			oldList[i] = i
		}
		sort.Slice(oldList, func(i, j int) bool {
			a, b := dl[oldList[i]], dl[oldList[j]]
			if a.priority != b.priority {
				return a.priority < b.priority
			}
			if a.syncGroup != b.syncGroup {
				return a.syncGroup > b.syncGroup
			}
			if a.syncId > 0 && a.syncId == b.syncId {
				if a.syncLayer != b.syncLayer {
					return a.syncLayer < b.syncLayer
				}
			}
			return a.sortindex > b.sortindex
		})
		newList := make([]int, n)
		for i := range newList {
			newList[i] = i
		}
		slices.SortFunc(newList, func(i, j int) int { return compareSpriteDraw(dl[i], dl[j]) })
		if !slices.Equal(oldList, newList) {
			t.Fatalf("draw list order %v, was %v", newList, oldList)
		}

		// Shadows: stable, by descending priority
		shadows := make(ShadowList, n)
		for i := range shadows {
			shadows[i] = &ShadowSprite{SpriteData: dl[i]}
		}
		oldShadows := slices.Clone(shadows)
		sort.SliceStable(oldShadows, func(i, j int) bool { return oldShadows[i].priority > oldShadows[j].priority })
		newShadows := slices.Clone(shadows)
		slices.SortStableFunc(newShadows, func(a, b *ShadowSprite) int { return cmp.Compare(b.priority, a.priority) })
		if !slices.Equal(oldShadows, newShadows) {
			t.Fatal("shadow order changed")
		}
	}
}
