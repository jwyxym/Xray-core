package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xtls/xray-core/common/cmdarg"
)

func setupConfigSets(t *testing.T, assignments ...string) {
	t.Helper()
	oldSets, oldFiles, oldDir, oldFormat := configSets, configFiles, configDir, *format
	t.Cleanup(func() {
		configSets, configFiles, configDir, *format = oldSets, oldFiles, oldDir, oldFormat
	})
	configSets, configFiles, configDir, *format = assignments, nil, "", "auto"
}

func TestRunConfigSetsWithoutFiles(t *testing.T) {
	setupConfigSets(t, "inbounds[0].protocol=socks", "inbounds[0].listen=127.0.0.1", "inbounds[0].port=1080", "outbounds[0].protocol=freedom")
	if files := getConfigFilePath(false); len(files) != 0 {
		t.Fatalf("--set-only mode must skip implicit config sources: %v", files)
	}
	config, err := loadRunConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Inbound) != 1 || len(config.Outbound) != 1 {
		t.Fatalf("unexpected built config: %v", config)
	}
}

func TestRunConfigSetsFileFormats(t *testing.T) {
	for _, test := range []struct {
		ext  string
		body string
	}{
		{"json", `{"log":{"loglevel":"debug"},"outbounds":[{"protocol":"freedom","tag":"keep"}]}`},
		{"jsonc", "{/* comment */\"log\":{\"loglevel\":\"debug\"},\"outbounds\":[{\"protocol\":\"freedom\",\"tag\":\"keep\"}]}"},
		{"yaml", "log:\n  loglevel: debug\noutbounds:\n  - protocol: freedom\n    tag: keep\n"},
		{"toml", "[log]\nloglevel = 'debug'\n[[outbounds]]\nprotocol = 'freedom'\ntag = 'keep'\n"},
	} {
		t.Run(test.ext, func(t *testing.T) {
			setupConfigSets(t, "log.loglevel=warning", "outbounds[0].tag=changed")
			file := filepath.Join(t.TempDir(), "config."+test.ext)
			if err := os.WriteFile(file, []byte(test.body), 0600); err != nil {
				t.Fatal(err)
			}
			files := cmdarg.Arg{file}
			data, err := configWithOverrides(files)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Log struct {
					Level string `json:"loglevel"`
				} `json:"log"`
				Outbounds []struct {
					Protocol string
					Tag      string
				} `json:"outbounds"`
			}
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if got.Log.Level != "warning" || len(got.Outbounds) != 1 || got.Outbounds[0].Protocol != "freedom" || got.Outbounds[0].Tag != "changed" {
				t.Fatalf("incorrect effective config: %s", data)
			}
			if _, err := loadRunConfig(files); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRunConfigSetsRejectProtobuf(t *testing.T) {
	setupConfigSets(t, "log.loglevel=warning")
	_, err := configWithOverrides(cmdarg.Arg{"config.pb"})
	if err == nil || !strings.Contains(err.Error(), "protobuf") {
		t.Fatalf("expected explicit protobuf error, got %v", err)
	}
}

func TestRunConfigSetsPreserveCustomJSON(t *testing.T) {
	setupConfigSets(t, "log.loglevel=warning")
	file := filepath.Join(t.TempDir(), "config.json")
	body := `{
		"dns":{"servers":["1.1.1.1"],"hosts":{"example.com":"127.0.0.1"}},
		"fakeDns":[{"ipPool":"198.18.0.0/15","poolSize":65535}],
		"outbounds":[{"protocol":"freedom"}]
	}`
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := configWithOverrides(cmdarg.Arg{file})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		DNS struct {
			Hosts map[string]string `json:"hosts"`
		} `json:"dns"`
		FakeDNS []struct {
			IPPool   string `json:"ipPool"`
			PoolSize int    `json:"poolSize"`
		} `json:"fakeDns"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.DNS.Hosts["example.com"] != "127.0.0.1" || len(got.FakeDNS) != 1 || got.FakeDNS[0].PoolSize != 65535 || got.FakeDNS[0].IPPool != "198.18.0.0/15" {
		t.Fatalf("custom config values lost during override: %s", data)
	}
	if _, err := loadRunConfig(cmdarg.Arg{file}); err != nil {
		t.Fatal(err)
	}
}
