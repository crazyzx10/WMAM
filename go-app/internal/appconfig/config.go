package appconfig

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Server ServerConfig
	Data   DataConfig
}

type ServerConfig struct {
	Host string
	Port int
}

type DataConfig struct {
	Dir string
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Host: "127.0.0.1",
			Port: 28384,
		},
		Data: DataConfig{
			Dir: "./data",
		},
	}
}

// Load reads the tiny config.yaml used by WMAM. It intentionally supports only
// the simple nested keys in config.yaml.example, keeping runtime config boring.
func Load(path string) (Config, error) {
	cfg := Default()

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	defer file.Close()

	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)

		switch section + "." + key {
		case "server.host":
			if value != "" {
				cfg.Server.Host = value
			}
		case "server.port":
			if port, err := strconv.Atoi(value); err == nil && port > 0 {
				cfg.Server.Port = port
			}
		case "data.dir":
			if value != "" {
				cfg.Data.Dir = value
			}
		}
	}

	return cfg, scanner.Err()
}
