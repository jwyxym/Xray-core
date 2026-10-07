package confargs

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/xtls/xray-core/common/cmdarg"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
)

// Merge preserves the normal file merge semantics, then applies CLI overrides.
func Merge(files []string, format string, assignments []string) ([]byte, error) {
	sources := make([]*core.ConfigSource, 0, len(files))
	for _, file := range files {
		f := format
		if f == "auto" {
			f = core.GetFormat(file)
			if file == "stdin:" {
				f = "json"
			}
		}
		switch f {
		case "json", "yaml", "toml":
		case "protobuf":
			return nil, fmt.Errorf("--set does not support protobuf config files")
		default:
			return nil, fmt.Errorf("failed to get config format of %s", file)
		}
		sources = append(sources, &core.ConfigSource{Name: file, Format: f})
	}
	base := []byte("{}")
	if len(sources) > 0 {
		var err error
		base, err = serial.MergeConfigJSON(sources)
		if err != nil {
			return nil, err
		}
	}
	return ApplySets(base, assignments)
}

// LoadArgs loads explicit config arguments without process-global flags, implicit
// config discovery, stdin reads, signal handlers or process exits.
func LoadArgs(args []string) (*core.Config, error) {
	flags := flag.NewFlagSet("xray-library", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var files, assignments cmdarg.Arg
	flags.Var(&files, "c", "Config file")
	flags.Var(&files, "config", "Config file")
	flags.Var(&assignments, "set", "Config override")
	dir := flags.String("confdir", "", "Config directory")
	format := flags.String("format", "auto", "Config format")
	if err := flags.Parse(args); err != nil {
		return nil, err
	}
	if flags.NArg() != 0 {
		return nil, fmt.Errorf("unexpected positional arguments; pass flags without the executable name or run command")
	}
	f := "auto"
	if *format != "auto" {
		f = core.GetFormatByExtension(*format)
		if f == "" {
			return nil, fmt.Errorf("unsupported config format %q", *format)
		}
	}
	if *dir != "" {
		entries, err := os.ReadDir(*dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			entryFormat := core.GetFormat(entry.Name())
			if !entry.IsDir() && (entryFormat == "json" || entryFormat == "yaml" || entryFormat == "toml") && (f == "auto" || entryFormat == f) {
				files = append(files, filepath.Join(*dir, entry.Name()))
			}
		}
	}
	if len(files) == 0 && len(assignments) == 0 {
		return nil, fmt.Errorf("provide at least one config file or --set assignment")
	}
	for _, file := range files {
		if file == "stdin:" {
			return nil, fmt.Errorf("stdin: is not supported by the library")
		}
	}
	if len(assignments) == 0 {
		return core.LoadConfig(f, files)
	}
	data, err := Merge(files, f, assignments)
	if err != nil {
		return nil, err
	}
	config, err := serial.DecodeJSONConfigStrict(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return config.Build()
}
