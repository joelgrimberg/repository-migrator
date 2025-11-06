package util

import "testing"

func TestEnvBool(t *testing.T) {
	cases := map[string]bool{
		"1": true, "true": true, "t": true, "yes": true, "y": true,
		"0": false, "false": false, "f": false, "no": false, "n": false,
	}
	for in, want := range cases {
		got, ok := EnvBool(in)
		if !ok { t.Fatalf("ok=false for %q", in) }
		if got != want { t.Fatalf("%q => %v want %v", in, got, want) }
	}
	if _, ok := EnvBool(""); ok {
		t.Fatalf("expected ok=false for empty string")
	}
}
