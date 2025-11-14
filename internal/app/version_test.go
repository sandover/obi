package app

import "testing"

func TestVersionIncludesSemver(t *testing.T) {
	origSemver := semver
	origSHA := gitSHA
	defer func() {
		semver = origSemver
		gitSHA = origSHA
	}()

	semver = "0.2.0"
	gitSHA = ""

	if got := Version(); got != "0.2.0" {
		t.Fatalf("expected semver only, got %q", got)
	}
}

func TestVersionIncludesGitSHA(t *testing.T) {
	origSemver := semver
	origSHA := gitSHA
	defer func() {
		semver = origSemver
		gitSHA = origSHA
	}()

	semver = "0.2.0"
	gitSHA = "abcdef123456"

	if got := Version(); got != "0.2.0 (abcdef1)" {
		t.Fatalf("expected semver + short sha, got %q", got)
	}
}
