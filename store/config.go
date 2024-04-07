package store

import (
	"fmt"
	"github.com/spf13/viper"
)

const (
	defaultDatabase = "otel"
	defaultTable    = "otel_traces"
)

type Config struct {
	DBHost                  string `yaml:"db_host"`
	DBPort                  int    `yaml:"db_port"`
	DBUser                  string `yaml:"db_user"`
	DBPass                  string `yaml:"db_pass"`
	DBName                  string `yaml:"db_name"`
	DBTable                 string `yaml:"db_table"`
	DBCaFile                string `yaml:"db_ca_file"`
	DBTlsEnabled            bool   `yaml:"db_tls_enabled"`
	DBTlsInsecure           bool   `yaml:"db_tls_insecure"`
	DBMaxOpenConns          uint   `yaml:"db_max_open_conns"`
	DBMaxIdleConns          uint   `yaml:"db_max_idle_conns"`
	DBConnMaxLifetimeMillis uint   `yaml:"db_conn_max_lifetime_millis"`
	DBConnMaxIdleTimeMillis uint   `yaml:"db_conn_max_idle_time_millis"`
	EnableTracing           bool   `yaml:"enable_tracing"`
}

func NewConfig(v *viper.Viper) (*Config, error) {
	config := &Config{}
	config.initFromViper(v)

	if err := config.validate(); err != nil {
		return nil, err
	}

	return config, nil
}

func (c *Config) initFromViper(v *viper.Viper) {
	c.DBHost = v.GetString("db_host")
	c.DBPort = v.GetInt("db_port")
	c.DBUser = v.GetString("db_user")
	c.DBPass = v.GetString("db_pass")
	c.DBName = v.GetString("db_name")
	c.DBTable = v.GetString("db_table")
	c.DBCaFile = v.GetString("db_ca_file")
	c.DBTlsEnabled = v.GetBool("db_tls_enabled")
	c.DBTlsInsecure = v.GetBool("db_tls_insecure")
	c.DBMaxOpenConns = v.GetUint("db_max_open_conns")
	c.DBMaxIdleConns = v.GetUint("db_max_idle_conns")
	c.DBConnMaxLifetimeMillis = v.GetUint("db_conn_max_lifetime_millis")
	c.DBConnMaxIdleTimeMillis = v.GetUint("db_conn_max_idle_time_millis")
	c.EnableTracing = v.GetBool("enable_tracing")
}

func (c *Config) validate() error {
	if c.DBHost == "" || c.DBPort == 0 {
		return fmt.Errorf("db_host and db_port must be set")
	}

	if c.DBUser == "" || c.DBPass == "" {
		return fmt.Errorf("db_user and db_pass must be set")
	}

	if c.DBName == "" {
		c.DBName = defaultDatabase
	}

	if c.DBTable == "" {
		c.DBTable = defaultTable
	}

	return nil
}
