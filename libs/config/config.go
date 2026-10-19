package config

import (
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/joeshaw/envdecode"
	_ "github.com/joho/godotenv/autoload"
)

type Config struct {
	Environment string `env:"ENVIRONMENT,default=development"`
	Server      ServerConfig
	DB          DBConfig
	Clinic      ClinicConfig
}

type ServerConfig struct {
	Host        string        `env:"SERVER_HOST,default=0.0.0.0"`
	Port        string        `env:"SERVER_PORT,default=8080"`
	TimeoutRead time.Duration `env:"SERVER_TIMEOUT_READ,default=10s"`
}

type DBConfig struct {
	Host     string `env:"DB_HOST,required"`
	Port     int    `env:"DB_PORT,required"`
	Username string `env:"DB_USER,required"`
	Password string `env:"DB_PASSWORD,required"`
	DBName   string `env:"DB_NAME,required"`
}

type ClinicConfig struct {
	Timezone       string `env:"CLINIC_TIMEZONE,default=Africa/Nairobi"`
	MinLeadMinutes int    `env:"MIN_LEAD_MINUTES,default=60"`
}


func (d DBConfig) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(d.Username, d.Password),
		Host:     fmt.Sprintf("%s:%d", d.Host, d.Port),
		Path:     d.DBName,
		RawQuery: "sslmode=disable",
	}
	return u.String()
}

func (c ClinicConfig) Location() (*time.Location, error) {
	return time.LoadLocation(c.Timezone)
}

func New() *Config {
	var c Config
	if err := envdecode.StrictDecode(&c); err != nil {
		log.Fatalf("Failed to decode config: %s", err)
	}
	return &c
}
