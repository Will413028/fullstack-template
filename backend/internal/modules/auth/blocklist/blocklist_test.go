package blocklist

import "testing"

func TestContains(t *testing.T) {
	// 5406 lines of at least 15 code points (the source list is in the package doc); NFKC folds none of them together.
	if n := Len(); n != 5406 {
		t.Errorf("Len = %d, want 5406", n)
	}
	if !Contains("demon1q2w3e4r5t") {
		t.Error("first entry not found")
	}
	if Contains("correct horse battery staple") {
		t.Error("a passphrase that is not on the list was found")
	}
}
