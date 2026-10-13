package main

import "testing"

// batchDraw queues one draw per layer, with its anims in the order they came
func TestAddBatchDraw(t *testing.T) {
	var groups []batchDrawGroup
	for i, layer := range []int16{1, 2, 1, -1, 2, 1} {
		groups = addBatchDraw(groups, layer, Anim{x: float32(i)}, 6)
	}
	want := map[int16][]float32{1: {0, 2, 5}, 2: {1, 4}, -1: {3}}
	if len(groups) != len(want) {
		t.Fatalf("%v groups, want %v", len(groups), len(want))
	}
	for i, layer := range []int16{1, 2, -1} {
		g := groups[i]
		if g.layer != layer || len(g.anims) != len(want[layer]) {
			t.Fatalf("group %v: layer %v with %v anims, want layer %v with %v", i, g.layer, len(g.anims), layer, len(want[layer]))
		}
		for j, x := range want[layer] {
			if g.anims[j].x != x {
				t.Errorf("layer %v, anim %v: x %v, want %v", layer, j, g.anims[j].x, x)
			}
		}
	}
	// Room for the whole batch from the start: anims are large, and copied on every growth
	if cap(groups[0].anims) != 6 {
		t.Errorf("first group made with room for %v anims, want 6", cap(groups[0].anims))
	}
}
