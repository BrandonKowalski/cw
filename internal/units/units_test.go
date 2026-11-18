package units

import "testing"

func TestSize(t *testing.T) {
	cases := map[int64]string{
		0:           "0 B",
		999:         "999 B",
		1000:        "1.0 kB",
		3300:        "3.3 kB",
		5000000:     "5.0 MB",
		-2500000000: "-2.5 GB",
		7 * 1e15:    "7000.0 TB",
	}
	for n, want := range cases {
		if got := Size(n); got != want {
			t.Errorf("Size(%d) = %q, want %q", n, got, want)
		}
	}
}
