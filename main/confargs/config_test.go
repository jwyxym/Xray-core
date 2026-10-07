package confargs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xtls/xray-core/main/confargs"
	_ "github.com/xtls/xray-core/main/distro/all"
)

func TestLoadArgs(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.json")
	if err := os.WriteFile(file, []byte(`{"outbounds":[{"protocol":"freedom","tag":"original"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--set", "outbounds[0].protocol=freedom", "--set", "outbounds[0].tag=changed"},
		{"-c", file, "--set=outbounds[0].tag=changed"},
		{"-confdir", dir, "--set", "outbounds[0].tag=changed"},
	} {
		config, err := confargs.LoadArgs(args)
		if err != nil {
			t.Fatal(err)
		}
		if len(config.Outbound) != 1 || config.Outbound[0].Tag != "changed" {
			t.Fatalf("incorrect effective config: %v", config)
		}
	}
	if _, err := confargs.LoadArgs([]string{"-c", file}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadArgsRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"run", "--set", "log.loglevel=warning"},
		{"--set"},
		{"--format", "invalid", "-c", "config.json"},
		{"-c", "stdin:"},
		{"--set", "log.loglevel=warning", "-c", "config.pb"},
		{"-dump"},
		{"-test"},
	} {
		if _, err := confargs.LoadArgs(args); err == nil {
			t.Fatalf("accepted invalid arguments: %v", args)
		}
	}
}
