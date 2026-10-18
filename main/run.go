package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/xtls/xray-core/common/cmdarg"
	"github.com/xtls/xray-core/common/errors"
	clog "github.com/xtls/xray-core/common/log"
	"github.com/xtls/xray-core/common/platform"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	"github.com/xtls/xray-core/main/commands/base"
	"github.com/xtls/xray-core/main/confargs"
)

var cmdRun = &base.Command{
	UsageLine: "{{.Exec}} run [-c config.json] [-confdir dir] [--set path=value]",
	Short:     "Run Xray with config, the default command",
	Long: `
Run Xray with config, the default command.

The -config=file, -c=file flags set the config files for
Xray. Multiple assign is accepted.

The -confdir=dir flag sets a dir with multiple json config

The -format=json flag sets the format of config files.
Default "auto".

The --set path=value flag overrides a config field after merging files.
Repeat it to set multiple fields; later values take precedence.
Paths use dots and zero-based array indices, for example inbounds[0].port=1080.
Array indices may replace existing elements or append at the current length.
Values are JSON when valid, otherwise plain strings. Quote numeric strings as JSON.
Without -c/-config or -confdir, --set starts with an empty config and skips
default files, environment config paths and stdin. Protobuf overrides are unsupported.
The -test and -dump flags use the final config, including all --set overrides.

The config root env object sets process environment variables after all config
files are parsed. Variables needed to locate or parse config files must be set
in the process environment before Xray starts.

The -test flag tells Xray to test config files only,
without launching the server.

The -dump flag tells Xray to print the merged config.
	`,
}

// The --lua-privileged-pubkey=key flag sets the trusted Ed25519 public key
// for privileged local Lua scripts, encoded as unpadded Base64URL.

func init() {
	cmdRun.Run = executeRun // break init loop
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
}

var (
	configFiles cmdarg.Arg // "Config file for Xray.", the option is customed type, parse in main
	configSets  cmdarg.Arg
	configDir   string
	dump        = cmdRun.Flag.Bool("dump", false, "Dump merged config only, without launching Xray server.")
	test        = cmdRun.Flag.Bool("test", false, "Test config file only, without launching Xray server.")
	format      = cmdRun.Flag.String("format", "auto", "Format of input file.")
	// TODO: Restore this flag and its help text when Lua privileged signature authorization is re-enabled.
	// luaPrivilegedPublicKey = cmdRun.Flag.String("lua-privileged-pubkey", "", "Unpadded Base64URL Ed25519 public key for privileged local Lua scripts.")

	/* We have to do this here because Golang's Test will also need to parse flag, before
	 * main func in this file is run.
	 */
	_ = func() bool {
		cmdRun.Flag.Var(&configFiles, "config", "Config path for Xray.")
		cmdRun.Flag.Var(&configFiles, "c", "Short alias of -config")
		cmdRun.Flag.Var(&configSets, "set", "Set a config field (path=value); may be repeated.")
		cmdRun.Flag.StringVar(&configDir, "confdir", "", "A dir with multiple json config")

		return true
	}()
)

func executeRun(cmd *base.Command, args []string) {
	if *dump {
		clog.ReplaceWithSeverityLogger(clog.Severity_Warning)
		errCode := dumpConfig()
		os.Exit(errCode)
	}

	printVersion()
	server, err := startXray()
	if err != nil {
		fmt.Println("Failed to start:", err)
		// Configuration error. Exit with a special value to prevent systemd from restarting.
		os.Exit(23)
	}

	if *test {
		fmt.Println("Configuration OK.")
		os.Exit(0)
	}

	if err := server.Start(); err != nil {
		fmt.Println("Failed to start:", err)
		os.Exit(-1)
	}
	defer server.Close()

	// Explicitly triggering GC to remove garbage from config loading.
	runtime.GC()
	debug.FreeOSMemory()

	{
		osSignals := make(chan os.Signal, 1)
		signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
		<-osSignals
	}
}

func dumpConfig() int {
	files := getConfigFilePath(false)
	if len(configSets) > 0 {
		config, err := configWithOverrides(files)
		if err != nil {
			fmt.Println(err)
			return 23
		}
		fmt.Println(string(config))
		return 0
	}
	if config, err := core.GetMergedConfig(files); err != nil {
		fmt.Println(err)
		time.Sleep(1 * time.Second)
		return 23
	} else {
		fmt.Print(config)
	}
	return 0
}

func fileExists(file string) bool {
	info, err := os.Stat(file)
	return err == nil && !info.IsDir()
}

func dirExists(file string) bool {
	if file == "" {
		return false
	}
	info, err := os.Stat(file)
	return err == nil && info.IsDir()
}

func getRegepxByFormat() string {
	switch strings.ToLower(*format) {
	case "json":
		return `^.+\.(json|jsonc)$`
	case "toml":
		return `^.+\.toml$`
	case "yaml", "yml":
		return `^.+\.(yaml|yml)$`
	default:
		return `^.+\.(json|jsonc|toml|yaml|yml)$`
	}
}

func readConfDir(dirPath string) {
	confs, err := os.ReadDir(dirPath)
	if err != nil {
		log.Fatalln(err)
	}
	for _, f := range confs {
		matched, err := regexp.MatchString(getRegepxByFormat(), f.Name())
		if err != nil {
			log.Fatalln(err)
		}
		if matched {
			configFiles.Set(path.Join(dirPath, f.Name()))
		}
	}
}

func getConfigFilePath(verbose bool) cmdarg.Arg {
	if len(configSets) > 0 && len(configFiles) == 0 && configDir == "" {
		return nil
	}
	if dirExists(configDir) {
		if verbose {
			log.Println("Using confdir from arg:", configDir)
		}
		readConfDir(configDir)
	} else if envConfDir := platform.GetConfDirPath(); dirExists(envConfDir) {
		if verbose {
			log.Println("Using confdir from env:", envConfDir)
		}
		readConfDir(envConfDir)
	}

	if len(configFiles) > 0 {
		return configFiles
	}

	if workingDir, err := os.Getwd(); err == nil {
		suffixes := []string{".json", ".jsonc", ".toml", ".yaml", ".yml"}
		for _, suffix := range suffixes {
			configFile := filepath.Join(workingDir, "config"+suffix)
			if fileExists(configFile) {
				if verbose {
					log.Println("Using default config: ", configFile)
				}
				return cmdarg.Arg{configFile}
			}
		}
	}

	if configFile := platform.GetConfigurationPath(); fileExists(configFile) {
		if verbose {
			log.Println("Using config from env: ", configFile)
		}
		return cmdarg.Arg{configFile}
	}

	if verbose {
		log.Println("Using config from STDIN")
	}
	return cmdarg.Arg{"stdin:"}
}

func getConfigFormat() string {
	f := core.GetFormatByExtension(*format)
	if f == "" {
		f = "auto"
	}
	return f
}

func startXray() (core.Server, error) {
	// TODO: Restore public key initialization when Lua privileged signature authorization is re-enabled.
	// if err := lua.SetPrivilegedPublicKey(*luaPrivilegedPublicKey); err != nil {
	// 	return nil, errors.New("failed to configure Lua privileged authorization").Base(err)
	// }

	configFiles := getConfigFilePath(true)

	c, err := loadRunConfig(configFiles)
	if err != nil {
		return nil, errors.New("failed to load config files: [", configFiles.String(), "]").Base(err)
	}

	server, err := core.New(c)
	if err != nil {
		return nil, errors.New("failed to create server").Base(err)
	}

	return server, nil
}

func loadRunConfig(files cmdarg.Arg) (*core.Config, error) {
	if len(configSets) == 0 {
		return core.LoadConfig(getConfigFormat(), files)
	}
	data, err := configWithOverrides(files)
	if err != nil {
		return nil, err
	}
	config, err := serial.DecodeJSONConfigStrict(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return config.Build()
}

func configWithOverrides(files cmdarg.Arg) ([]byte, error) {
	return confargs.Merge(files, getConfigFormat(), configSets)
}
