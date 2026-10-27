package enrichment

import "testing"

func TestValidators(t *testing.T) {
	cases := []struct {
		name  string
		issue string
		valid bool
	}{
		{"email ok", Email("a@b.pe"), true},
		{"email bad", Email("not-an-email"), false},
		{"email display name", Email("Joe <a@b.pe>"), false},
		{"phone ok", Phone("+51 999 888 777"), true},
		{"phone letters", Phone("abc"), false},
		{"ubigeo ok", Ubigeo("150101"), true},
		{"ubigeo short", Ubigeo("1501"), false},
		{"date ok", BirthDate("1990-05-17"), true},
		{"date format", BirthDate("17/05/1990"), false},
		{"date future", BirthDate("2999-01-01"), false},
		{"date too old", BirthDate("1800-01-01"), false},
		{"sex ok", Sex("female"), true},
		{"sex bad", Sex("F"), false},
		{"text empty", Text("  ", 10), false},
		{"text long", Text("12345678901", 10), false},
		{"text ok", Text("Lima", 10), true},
	}
	for _, c := range cases {
		if got := c.issue == ""; got != c.valid {
			t.Errorf("%s: valid=%v, issue=%q", c.name, got, c.issue)
		}
	}
}

func TestCollectSkipsUnsentFields(t *testing.T) {
	bad := "nope"
	details := Collect(Run("email", nil, Email), Run("phone", &bad, Phone))
	if len(details) != 1 || details[0].Field != "phone" {
		t.Fatalf("unexpected details: %+v", details)
	}
}
