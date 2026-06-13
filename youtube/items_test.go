package youtube

import "testing"

func TestItemSelector(t *testing.T) {
	cases := []struct {
		spec  string
		total int
		want  map[int]bool // index -> selected
	}{
		{"", 10, map[int]bool{1: true, 5: true, 10: true}},
		{"1,3,5", 10, map[int]bool{1: true, 2: false, 3: true, 4: false, 5: true}},
		{"3-5", 10, map[int]bool{2: false, 3: true, 4: true, 5: true, 6: false}},
		{"5-", 10, map[int]bool{4: false, 5: true, 9: true, 10: true}},
		{"-3", 10, map[int]bool{8: true, 7: false, 9: false}}, // 3rd from the end
		{"-1", 10, map[int]bool{10: true, 9: false}},          // last
		{"1,8-", 10, map[int]bool{1: true, 7: false, 8: true, 10: true}},
	}
	for _, c := range cases {
		sel, err := ParseItemSelector(c.spec)
		if err != nil {
			t.Fatalf("%q: %v", c.spec, err)
		}
		for idx, want := range c.want {
			if got := sel.Selects(idx, c.total); got != want {
				t.Errorf("spec %q index %d (total %d): got %v want %v", c.spec, idx, c.total, got, want)
			}
		}
	}
}
