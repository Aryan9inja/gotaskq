package metrics

import "testing"

func TestNormalizeLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  MyQueue  ", "myqueue"},
		{"JobType", "jobtype"},
		{"   ", "unknown"},
		{"", "unknown"},
	}

	for _, test := range tests {
		result := normalizeLabel(test.input)
		if result != test.expected {
			t.Errorf("normalizeLabel(%q) = %q; want %q", test.input, result, test.expected)
		}
	}
}