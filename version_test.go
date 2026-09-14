package svcforge

import "testing"

func TestCompareVersionsSemVerPrecedence(t *testing.T) {
	ordered := []string{
		"1.0.0-alpha",
		"1.0.0-alpha.1",
		"1.0.0-alpha.beta",
		"1.0.0-beta",
		"1.0.0-beta.2",
		"1.0.0-beta.11",
		"1.0.0-rc.1",
		"1.0.0",
	}
	for i := 0; i < len(ordered)-1; i++ {
		got, err := CompareVersions(ordered[i], ordered[i+1])
		if err != nil {
			t.Fatal(err)
		}
		if got >= 0 {
			t.Fatalf("expected %s < %s, got %d", ordered[i], ordered[i+1], got)
		}
	}
	if got, err := CompareVersions("v1.2.3+build.1", "1.2.3+build.2"); err != nil || got != 0 {
		t.Fatalf("build metadata precedence: got=%d err=%v", got, err)
	}
}

func TestParseVersionRejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{"", "1", "1.2", "01.2.3", "1.02.3", "1.2.03", "1.2.3-01", "1.2.3+"} {
		if _, err := ParseVersion(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}
