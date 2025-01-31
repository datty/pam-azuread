package conf

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

const configFile = "/etc/azuread.conf"
const configFileSecrets = "/etc/azuread-secret.conf"

// config define azureAD parameters
// and setting for this module
type Config struct {
	ClientID     string `yaml:"client-id"`
	ClientSecret string `yaml:"client-secret"`
	RedirectURL  string `yaml:"redirect-url"`
	TenantID     string `yaml:"tenant-id"`
	Domain       string `yaml:"o365-domain"`
	//Used for lookup of user UID from AzureAD Custom Security Attributes
	UseSecAttributes  bool   `yaml:"custom-security-attributes"`
	AttributeSet      string `yaml:"attribute-set"`
	UserUIDAttribute  string `yaml:"user-uid-attribute-name"`
	UserGIDAttribute  string `yaml:"user-gid-attribute-name"`
	UserDefaultGID    uint   `yaml:"user-gid-default"`
	UserAutoUID       bool   `yaml:"user-auto-uid"`
	MinUID            int    `yaml:"uid-range-min"`
	MaxUID            int    `yaml:"uid-range-max"`
	GroupGidAttribute string `yaml:"group-gid-attribute-name"`
	GroupAutoGID      bool   `yaml:"group-auto-gid"`
	MinGID            int    `yaml:"gid-range-min"`
	MaxGID            int    `yaml:"gid-range-max"`
	//Should not need to change these...
	PamScopes []string `yaml:"pam-scopes"`
	NssScopes []string `yaml:"nss-scopes"`
}
type ConfigSecrets struct {
	ClientID     string `yaml:"client-id"`
	ClientSecret string `yaml:"client-secret"`
}

// ReadConfig
// need file path from yaml and return config
func ReadConfig() (*Config, error) {
	yamlFile, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	var c Config
	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal filecontent to config struct:%w", err)
	}
	return &c, nil
}
func ReadSecrets() (*ConfigSecrets, error) {
	yamlFile, err := os.ReadFile(configFileSecrets)
	if err != nil {
		return nil, err
	}
	var c ConfigSecrets
	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal filecontent to config struct:%w", err)
	}
	return &c, nil
}
