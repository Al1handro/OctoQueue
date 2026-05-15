package config

import (
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env string `yaml:"env" env-default:"local"`
	HttpServer `yaml:"http_server"`
	PsqlInfo `yaml:"psql_info"`
}

type HttpServer struct {
	Adress string `yaml:"addres" env-default:"localhost:8080"`
	Timeout time.Duration `yaml:"timeout" env-default:"4s"`
	IdleTimeout time.Duration `yaml:"idle_timeout" env-default:"60s"`
}

type PsqlInfo struct {
	Рost     string `yaml:"host" env-required:"true" env-default:"localhost"`
	Port     string `yaml:"port" env-required:"true" env-default:"5432"`
	User     string `yaml:"user" env-required:"true" env-default:"postgres"`
	Password string `yaml:"password" env-required:"true" env-default:"password"`
	Dbname   string `yaml:"dbname" env-required:"true" env-default:"app"`
}

 func MustLode() *Config {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		log.Fatal("CONFIG_PATH is not set")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("configPath is not exist: %s", configPath)
	}

 	var cnf Config

	if err := cleanenv.ReadConfig(configPath, &cnf); err != nil {
		log.Fatalf("cannot reade config: %s", err)
	}

	return &cnf
}