package main

import (
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestLimiterConfiguration(t *testing.T) {
	// Re-enter only this test in a subprocess because main calls os.Exit.
	if os.Getenv("WEBHOOKER_TEST_CONFIG_CHILD") == "1" {
		var args []string
		if err := json.Unmarshal([]byte(os.Getenv("WEBHOOKER_TEST_CONFIG_ARGS")), &args); err != nil {
			panic(err)
		}
		os.Args = append([]string{"webhooker"}, args...)
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		main()
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"zero rate", []string{"-limiter-rps=0"}, "limiter-rps must be finite and greater than zero"},
		{"negative rate", []string{"-limiter-rps=-1"}, "limiter-rps must be finite and greater than zero"},
		{"nan rate", []string{"-limiter-rps=NaN"}, "limiter-rps must be finite and greater than zero"},
		{"infinite rate", []string{"-limiter-rps=+Inf"}, "limiter-rps must be finite and greater than zero"},
		{"zero burst", []string{"-limiter-burst=0"}, "limiter-burst must be at least 1"},
		{"negative burst", []string{"-limiter-burst=-1"}, "limiter-burst must be at least 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args, err := json.Marshal(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(executable, "-test.run=^TestLimiterConfiguration$")
			cmd.Env = append(os.Environ(), "WEBHOOKER_TEST_CONFIG_CHILD=1", "WEBHOOKER_TEST_CONFIG_ARGS="+string(args), "WEBHOOKER_DB_DSN=")
			out, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 || !strings.Contains(string(out), tc.want) {
				t.Fatalf("startup error=%v output=%s; want %s", err, out, tc.want)
			}
		})
	}
}
