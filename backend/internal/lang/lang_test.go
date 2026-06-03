package lang

import "testing"

func TestFromAccept(t *testing.T) {
	cases := map[string]string{
		"fr-FR,fr;q=0.9,en;q=0.8": "fr",
		"pt-BR":                   "pt",
		"  EN ":                   "en",
		"":                        "",
		"es":                      "es",
	}
	for in, want := range cases {
		if got := FromAccept(in); got != want {
			t.Errorf("FromAccept(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPick_Precedence(t *testing.T) {
	if got := Pick("de", "fr,en", "en"); got != "de" {
		t.Errorf("override should win, got %q", got)
	}
	if got := Pick("", "fr-FR,fr;q=0.9", "en"); got != "fr" {
		t.Errorf("header next, got %q", got)
	}
	if got := Pick("", "", "en"); got != "en" {
		t.Errorf("fallback last, got %q", got)
	}
}

func TestResolve_R9(t *testing.T) {
	cases := []struct {
		name         string
		visitor      string
		a, b         []string
		wantDisplay  string
		wantFellBack bool
	}{
		{
			"both have visitor lang", "fr",
			[]string{"fr", "en"}, []string{"fr", "en", "de"}, "fr", false,
		},
		{
			"one lacks it → English both", "fr",
			[]string{"fr", "en"}, []string{"en", "de"}, "en", true,
		},
		{
			"no preference → fallback, no note", "",
			[]string{"en"}, []string{"en"}, "en", false,
		},
		{
			"visitor already English", "en",
			[]string{"en"}, []string{"en"}, "en", false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, fb := Resolve(c.visitor, "en", c.a, c.b)
			if d != c.wantDisplay || fb != c.wantFellBack {
				t.Errorf("Resolve = (%q,%v), want (%q,%v)",
					d, fb, c.wantDisplay, c.wantFellBack)
			}
		})
	}
}
