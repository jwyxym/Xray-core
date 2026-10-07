package confargs

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplyConfigSets(t *testing.T) {
	base := []byte(`{"inbounds":[{"port":80,"tag":"keep"}],"large":9007199254740993}`)
	result, err := ApplySets(base, []string{
		"inbounds[0].port=1080",
		"inbounds[0].settings.udp=true",
		"inbounds[1].protocol=socks",
		"inbounds[1].port=1081",
		"log.loglevel=debug",
		"log.loglevel=warning",
		`dns.servers=["1.1.1.1","8.8.8.8"]`,
		`outbounds=[{"protocol":"freedom"}]`,
		`env.NUMERIC="123"`,
		"env.EMPTY=",
		"env.EQUALS=a=b=c",
		"optional=null",
		"matrix[0][0]=42",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(result)))
	decoder.UseNumber()
	if err := decoder.Decode(&got); err != nil {
		t.Fatal(err)
	}
	inbounds := got["inbounds"].([]any)
	first := inbounds[0].(map[string]any)
	if first["port"] != json.Number("1080") || first["tag"] != "keep" || first["settings"].(map[string]any)["udp"] != true {
		t.Fatalf("incorrect inbound override: %s", result)
	}
	if len(inbounds) != 2 || inbounds[1].(map[string]any)["port"] != json.Number("1081") {
		t.Fatalf("incorrect appended inbound: %s", result)
	}
	if got["large"] != json.Number("9007199254740993") || got["log"].(map[string]any)["loglevel"] != "warning" {
		t.Fatalf("number precision or assignment order lost: %s", result)
	}
	env := got["env"].(map[string]any)
	if env["NUMERIC"] != "123" || env["EMPTY"] != "" || env["EQUALS"] != "a=b=c" {
		t.Fatalf("incorrect string values: %s", result)
	}
	if got["optional"] != nil || got["matrix"].([]any)[0].([]any)[0] != json.Number("42") {
		t.Fatalf("incorrect null or nested array: %s", result)
	}
	if got["dns"].(map[string]any)["servers"].([]any)[1] != "8.8.8.8" || got["outbounds"].([]any)[0].(map[string]any)["protocol"] != "freedom" {
		t.Fatalf("incorrect JSON values: %s", result)
	}
}

func TestApplyConfigSetsErrors(t *testing.T) {
	for _, assignment := range []string{
		"missing-equals", "=1", ".port=1", "inbounds.=1", "inbounds..port=1",
		"[0].port=1", "inbounds[-1].port=1", "inbounds[].port=1", "inbounds[0=1",
		"inbounds[0]port=1", "inbounds[0]]=1", "inbounds[999999999999999999999]=1",
		"inbounds[1].port=1", "log[0]=1", "scalar.child=1",
	} {
		t.Run(assignment, func(t *testing.T) {
			_, err := ApplySets([]byte(`{"log":{},"scalar":1}`), []string{assignment})
			if err == nil {
				t.Fatal("expected invalid path or type error")
			}
		})
	}
}

func TestApplyConfigSetsFromEmpty(t *testing.T) {
	result, err := ApplySets([]byte(`{}`), []string{
		"inbounds[0].protocol=socks", "inbounds[0].port=1080", "outbounds[0].protocol=freedom",
	})
	if err != nil || !json.Valid(result) {
		t.Fatalf("cannot build config from flags: %s, %v", result, err)
	}
}
