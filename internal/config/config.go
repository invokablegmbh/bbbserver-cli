package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	EnvBaseURL = "BBBSERVER_BASE_URL"
	EnvAPIKey  = "BBBSERVER_API_KEY"
	EnvTimeout = "BBBSERVER_TIMEOUT"
)

type Settings struct {
	BaseURL  string        `mapstructure:"base_url" json:"base_url" yaml:"base_url"`
	APIKey   string        `mapstructure:"api_key" json:"api_key" yaml:"api_key"`
	Timeout  time.Duration `mapstructure:"timeout" json:"timeout" yaml:"timeout"`
	Output   string        `mapstructure:"output" json:"output" yaml:"output"`
	Pretty   bool          `mapstructure:"pretty" json:"pretty" yaml:"pretty"`
	Verbose  bool          `mapstructure:"verbose" json:"verbose" yaml:"verbose"`
	Debug    bool          `mapstructure:"debug" json:"debug" yaml:"debug"`
	AuthMode string        `mapstructure:"auth_mode" json:"auth_mode" yaml:"auth_mode"`
}

type LoadOptions struct {
	ConfigPath string

	BaseURL  *string
	APIKey   *string
	Timeout  *time.Duration
	Output   *string
	Pretty   *bool
	Verbose  *bool
	Debug    *bool
	AuthMode *string
}

func DefaultConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "bbbserver-cli", "config.yaml"), nil
}

func Load(opts LoadOptions) (Settings, error) {
	configPath := opts.ConfigPath
	if configPath == "" {
		var err error
		configPath, err = DefaultConfigPath()
		if err != nil {
			return Settings{}, err
		}
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	v.SetDefault("base_url", "https://app.bbbserver.de/bbb-system-api")
	v.SetDefault("api_key", "")
	v.SetDefault("timeout", "30s")
	v.SetDefault("output", "human")
	v.SetDefault("pretty", false)
	v.SetDefault("verbose", false)
	v.SetDefault("debug", false)
	v.SetDefault("auth_mode", "apikey")

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !os.IsNotExist(err) {
			return Settings{}, fmt.Errorf("read config file: %w", err)
		}
	}

	v.SetEnvPrefix("BBBSERVER")
	v.AutomaticEnv()

	if opts.BaseURL != nil {
		v.Set("base_url", *opts.BaseURL)
	}
	if opts.APIKey != nil {
		v.Set("api_key", *opts.APIKey)
	}
	if opts.Timeout != nil {
		v.Set("timeout", opts.Timeout.String())
	}
	if opts.Output != nil {
		v.Set("output", *opts.Output)
	}
	if opts.Pretty != nil {
		v.Set("pretty", *opts.Pretty)
	}
	if opts.Verbose != nil {
		v.Set("verbose", *opts.Verbose)
	}
	if opts.Debug != nil {
		v.Set("debug", *opts.Debug)
	}
	if opts.AuthMode != nil {
		v.Set("auth_mode", *opts.AuthMode)
	}

	settings := Settings{
		BaseURL:  strings.TrimSpace(v.GetString("base_url")),
		APIKey:   strings.TrimSpace(v.GetString("api_key")),
		Output:   strings.TrimSpace(v.GetString("output")),
		Pretty:   v.GetBool("pretty"),
		Verbose:  v.GetBool("verbose"),
		Debug:    v.GetBool("debug"),
		AuthMode: strings.TrimSpace(v.GetString("auth_mode")),
	}

	timeoutStr := strings.TrimSpace(v.GetString("timeout"))
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return Settings{}, fmt.Errorf("invalid timeout %q: %w", timeoutStr, err)
	}
	settings.Timeout = timeout

	if settings.Output == "" {
		settings.Output = "human"
	}
	if settings.AuthMode == "" {
		settings.AuthMode = "apikey"
	}

	return settings, nil
}

func Init(configPath string) (string, error) {
	if configPath == "" {
		var err error
		configPath, err = DefaultConfigPath()
		if err != nil {
			return "", err
		}
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	if _, err := os.Stat(configPath); err == nil {
		return configPath, nil
	}

	v := viper.New()
	v.Set("base_url", "https://app.bbbserver.de/bbb-system-api")
	v.Set("api_key", "")
	v.Set("timeout", "30s")
	v.Set("auth_mode", "apikey")

	if err := v.WriteConfigAs(configPath); err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}

	if err := os.Chmod(configPath, 0o600); err != nil {
		return "", fmt.Errorf("secure config permissions: %w", err)
	}

	return configPath, nil
}

func SetValue(configPath, key, value string) (string, error) {
	if configPath == "" {
		var err error
		configPath, err = DefaultConfigPath()
		if err != nil {
			return "", err
		}
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !os.IsNotExist(err) {
			return "", fmt.Errorf("read config: %w", err)
		}
	}

	v.Set(key, value)
	if _, err := os.Stat(configPath); err == nil {
		if err := v.WriteConfig(); err != nil {
			return "", fmt.Errorf("write config: %w", err)
		}
	} else {
		if err := v.WriteConfigAs(configPath); err != nil {
			return "", fmt.Errorf("write config: %w", err)
		}
	}

	if err := os.Chmod(configPath, 0o600); err != nil {
		return "", fmt.Errorf("secure config permissions: %w", err)
	}

	return configPath, nil
}

func MaskAPIKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(trimmed)-4) + trimmed[len(trimmed)-4:]
}

func MaskedMap(s Settings, configPath string) map[string]any {
	return map[string]any{
		"base_url":    s.BaseURL,
		"api_key":     MaskAPIKey(s.APIKey),
		"timeout":     s.Timeout.String(),
		"output":      s.Output,
		"pretty":      s.Pretty,
		"verbose":     s.Verbose,
		"debug":       s.Debug,
		"auth_mode":   s.AuthMode,
		"config_path": configPath,
	}
}
