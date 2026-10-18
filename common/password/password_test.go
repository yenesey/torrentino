package password

import (
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	encoded, err := Hash("correct horse battery staple", MinIterations)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "pbkdf2-sha256$100000$") {
		t.Errorf("hash = %q, want the documented format", encoded)
	}
	if !Verify("correct horse battery staple", encoded) {
		t.Error("Verify rejected the correct password")
	}
	if Verify("wrong", encoded) {
		t.Error("Verify accepted a wrong password")
	}
}

func TestHashIsSalted(t *testing.T) {
	first, err := Hash("same", MinIterations)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Hash("same", MinIterations)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Error("two hashes of the same password are identical")
	}
}

func TestParseRejectsMalformedHashes(t *testing.T) {
	for _, value := range []string{
		"",
		"plain",
		"pbkdf2-sha256$100000$only-three",
		"pbkdf2-sha256$abc$c2FsdA==$a2V5",
		"pbkdf2-sha256$99999$c2FsdHNhbHRzYWx0c2FsdA==$a2V5",
		"pbkdf2-sha256$600000$!!!$a2V5",
		"md5$600000$c2FsdA==$a2V5",
	} {
		if err := Validate(value); err == nil {
			t.Errorf("Validate(%q) = nil, want an error", value)
		}
	}
}

func TestHashIterationsBounds(t *testing.T) {
	for _, iterations := range []int{0, MinIterations - 1, MaxIterations + 1} {
		if _, err := Hash("x", iterations); err == nil {
			t.Errorf("Hash with %d iterations succeeded", iterations)
		}
	}
}

func TestVerifyMalformedIsFalseAndDoesNotPanic(t *testing.T) {
	if Verify("x", "nonsense") {
		t.Error("Verify accepted a malformed hash")
	}
}
