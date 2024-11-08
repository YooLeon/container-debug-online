package compose

import (
	"io/ioutil"

	"gopkg.in/yaml.v3"
)

type ComposeConfig struct {
	Services map[string]Service `yaml:"services"`
}

type Service struct {
	Image       string   `yaml:"image"`
	Deploy      Deploy   `yaml:"deploy"`
	Environment []string `yaml:"environment"`
}

type Deploy struct {
	Resources Resources `yaml:"resources"`
}

type Resources struct {
	Reservations Reservations `yaml:"reservations"`
}

type Reservations struct {
	Devices []Device `yaml:"devices"`
}

type Device struct {
	Driver       string   `yaml:"driver"`
	Count        int      `yaml:"count"`
	Capabilities []string `yaml:"capabilities"`
}

func ParseComposeFile(path string) (*ComposeConfig, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := &ComposeConfig{}
	err = yaml.Unmarshal(data, config)
	return config, err
}
