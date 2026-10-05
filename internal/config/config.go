package config

import (
	"fmt"
	"os"
	"reflect"

	"github.com/mtariq99/retrievion/models"
	"github.com/spf13/viper"
)

var Cfg models.Config

// bindEnvs walks a struct type and registers every mapstructure-tagged
// leaf field with viper.BindEnv. This is required because AutomaticEnv
// only resolves keys Viper already knows about (from a loaded config
// file, a default, or an explicit bind) — it does not scan os.Environ()
// against arbitrary struct tags. In containers there is intentionally
// no config file on disk, so without this, Unmarshal silently leaves
// every field at its zero value even though the OS environment has
// the real value. The bind list is derived from models.Config itself,
// so it can never drift from the schema.
func bindEnvs(t reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		ft := field.Type

		if ft.Kind() == reflect.Struct {
			bindEnvs(ft)
			continue
		}

		tag := field.Tag.Get("mapstructure")
		if tag == "" || tag == ",squash" {
			continue
		}

		_ = viper.BindEnv(tag)
	}
}

func Load() (*models.Config, error) {
	wd, _ := os.Getwd()
	fmt.Println("Working directory:", wd)

	env := os.Getenv("ENV")

	var configFile string
	switch env {
	case "prod":
		configFile = ".env.prod"
	case "staging":
		configFile = ".env.staging"
	default:
		configFile = ".env"
	}

	viper.SetConfigFile(configFile)
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	bindEnvs(reflect.TypeOf(models.Config{}))

	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("No config file found (%s), using environment variables only\n", configFile)
	} else {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	if err := viper.Unmarshal(&Cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Defaults.
	if Cfg.Server.Port == "" {
		Cfg.Server.Port = "8080"
	}

	if Cfg.Server.AllowedOrigins == "" {
		Cfg.Server.AllowedOrigins = "http://localhost:3000"
	}

	if Cfg.Database.MaxOpenConns == 0 {
		Cfg.Database.MaxOpenConns = 50
	}

	if Cfg.Database.MaxIdleConns == 0 {
		Cfg.Database.MaxIdleConns = 10
	}

	if Cfg.Database.MigrationPath == "" {
		Cfg.Database.MigrationPath = "migrations"
	}

	if Cfg.LLM.MaxContextTokens == 0 {
		Cfg.LLM.MaxContextTokens = 8000
	}

	// Required configuration.
	if Cfg.Database.URL == "" && Cfg.Database.DSN == "" {
		return nil, fmt.Errorf(
			"DATABASE_URL or DATABASE_DSN is required but not set",
		)
	}

	return &Cfg, nil
}
