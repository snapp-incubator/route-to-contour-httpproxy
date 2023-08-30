package config

import (
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

type DefaultTimeout struct {
	PublicClass  string `koanf:"publicClass"`
	InterDcClass string `koanf:"interDcClass"`
	DefaultClass string `koanf:"defaultClass"`
}

type Config struct {
	// The ratio of the count of router nodes to the count of contour nodes
	RouterToContourRatio int `koanf:"routeToContourRatio"`

	// When using the host of a route as the name of the httproxy,
	// CommonHostSuffix will be removed from its end if present
	CommonHostSuffix string `koanf:"commonHostSuffix"`

	DefaultTimeout DefaultTimeout `koanf:"defaultTimeout"`
}

var (
	defaultConfig = Config{
		RouterToContourRatio: 1,
		CommonHostSuffix:     ".okd4.ts-1.staging-snappcloud.io",
		DefaultTimeout: DefaultTimeout{
			PublicClass:  "5s",
			InterDcClass: "5s",
			DefaultClass: "30s",
		},
	}
)

func GetConfig(configPath string) (*Config, error) {
	k := koanf.New(".")
	parser := yaml.Parser()
	cfg := &Config{}

	if err := k.Load(structs.Provider(defaultConfig, "koanf"), nil); err != nil {
		return nil, err
	}

	if err := k.Load(file.Provider(configPath), parser); err != nil {
		return nil, err
	}

	if err := k.Unmarshal("", cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
