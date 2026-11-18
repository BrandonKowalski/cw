package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Library            string `toml:"library"`
	TrackWarnThreshold int    `toml:"track_warn_threshold"`
	Jobs               int    `toml:"jobs"`
}

func defaults() Config {
	return Config{TrackWarnThreshold: 20000, Jobs: runtime.NumCPU()}
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "cw", "config.toml"), nil
}

func Load(path string) (Config, error) {
	c := defaults()
	if _, err := toml.DecodeFile(path, &c); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c, nil
		}
		return Config{}, err
	}
	if c.Library == "~" || strings.HasPrefix(c.Library, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return Config{}, err
		}
		c.Library = filepath.Join(home, strings.TrimPrefix(c.Library, "~"))
	}
	return c, nil
}
