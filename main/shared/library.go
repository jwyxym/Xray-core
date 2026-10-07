package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	"github.com/xtls/xray-core/main/confargs"
	_ "github.com/xtls/xray-core/main/distro/all"
)

// One instance per loaded library: several core services use process-global state.
type library struct {
	mu        sync.Mutex
	instance  *core.Instance
	lastError string
}

var shared library

func (l *library) operation(fn func() error) (status int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	defer func() {
		if recovered := recover(); recovered != nil {
			l.lastError = fmt.Sprintf("xray operation panicked: %v", recovered)
			status = -1
		}
	}()
	if err := fn(); err != nil {
		l.lastError = err.Error()
		return -1
	}
	l.lastError = ""
	return 0
}

func (l *library) start(load func() (*core.Config, error)) int {
	return l.operation(func() error {
		if l.instance != nil {
			return errors.New("Xray is already running; stop it before starting again")
		}
		config, err := load()
		if err != nil {
			return err
		}
		instance, err := core.New(config)
		if err != nil {
			return err
		}
		started := false
		defer func() {
			if !started {
				_ = instance.Close()
			}
		}()
		if err := instance.Start(); err != nil {
			return err
		}
		l.instance = instance
		started = true
		return nil
	})
}

func (l *library) startJSON(text string) int {
	return l.start(func() (*core.Config, error) {
		data := bytes.TrimSpace([]byte(text))
		if !json.Valid(data) || data[0] != '{' {
			return nil, errors.New("config must be a JSON object")
		}
		config, err := serial.DecodeJSONConfigStrict(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return config.Build()
	})
}

func (l *library) startArgs(text string) int {
	return l.start(func() (*core.Config, error) {
		var args []string
		if err := json.Unmarshal([]byte(text), &args); err != nil {
			return nil, fmt.Errorf("arguments must be a JSON string array: %w", err)
		}
		return confargs.LoadArgs(args)
	})
}

func (l *library) stop() int {
	return l.operation(func() error {
		if l.instance == nil {
			return nil
		}
		if err := l.instance.Close(); err != nil {
			return err
		}
		l.instance = nil
		return nil
	})
}

func (l *library) errorText() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lastError
}

func main() {}
