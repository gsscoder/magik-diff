package checks

import "testing"

func TestList_EveryCheckIsComplete(t *testing.T) {
	for _, c := range List() {
		if c.Name == "" || c.Description == "" || c.Prompt == "" {
			t.Errorf("built-in check %q has an empty field: %+v", c.Name, c)
		}
	}
}

func TestList_NamesAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, c := range List() {
		if seen[c.Name] {
			t.Errorf("built-in check name %q is used more than once", c.Name)
		}
		seen[c.Name] = true
	}
}

func TestFind_KnownName(t *testing.T) {
	c, ok := Find("language-consistency")
	if !ok {
		t.Fatal(`Find("language-consistency"): got false, want true`)
	}
	if c.Name != "language-consistency" {
		t.Errorf("Find returned check named %q, want %q", c.Name, "language-consistency")
	}
}

func TestFind_UnknownName(t *testing.T) {
	if c, ok := Find("no-such-check"); ok {
		t.Errorf(`Find("no-such-check") = %+v, true; want false`, c)
	}
}

func TestList_ReturnsCopy(t *testing.T) {
	got := List()
	if len(got) == 0 {
		t.Fatal("List returned no built-in checks")
	}
	name := got[0].Name
	got[0].Prompt = "mutated"

	if List()[0].Prompt == "mutated" {
		t.Error("mutating a List result changed a later List result")
	}
	if c, _ := Find(name); c.Prompt == "mutated" {
		t.Error("mutating a List result changed a later Find result")
	}
}
