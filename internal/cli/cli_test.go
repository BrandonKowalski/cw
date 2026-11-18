package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/BrandonKowalski/cw/internal/cli"
)

func TestRunUnknownCommand(t *testing.T) {
	var out, errb bytes.Buffer
	if code := cli.Run([]string{"bogus"}, nil, &out, &errb); code != 2 {
		t.Fatal(code)
	}
	if !strings.Contains(errb.String(), "usage: cw") {
		t.Fatal(errb.String())
	}
}
