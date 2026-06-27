package runner

import "testing"

func TestQuote(t *testing.T) {
	cases := map[string]string{
		`hi`:         `"hi"`,
		`a "b" c`:    `"a \"b\" c"`,
		`back\slash`: `"back\\slash"`,
	}
	for in, want := range cases {
		if got := quote(in); got != want {
			t.Fatalf("quote(%q) = %q, want %q", in, got, want)
		}
	}
}
