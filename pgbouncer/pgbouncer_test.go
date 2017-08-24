package pgbouncer

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateConfig_WithInvalidConfigTemplateErrors(t *testing.T) {
	bouncer := PGBouncer{
		ConfigFile:         "/etc/pgbouncer/pgbouncer.ini",
		ConfigFileTemplate: "/this/does/not/exist",
	}

	err := bouncer.GenerateConfig("curly.db.ams.gc.cx")
	if assert.Error(t, err, "expected config generation to fail") {
		assert.Regexp(t, "failed to read PGBouncer config template file", err.Error())
	}
}

func TestGenerateConfig_WritesConfigWithHost(t *testing.T) {
	tempConfigFile := makeTempConfigFile(t)
	defer os.Remove(tempConfigFile.Name())

	bouncer := PGBouncer{
		ConfigFile:         tempConfigFile.Name(),
		ConfigFileTemplate: "./fixtures/pgbouncer.ini.template",
	}

	err := bouncer.GenerateConfig("curly.db.ams.gc.cx")
	assert.Nil(t, err, "failed to generate config")

	configBuffer, _ := ioutil.ReadFile(tempConfigFile.Name())
	assert.Contains(t, string(configBuffer),
		"postgres = host=curly.db.ams.gc.cx", "expected host to be in generated config")
}

func makeTempConfigFile(t *testing.T) *os.File {
	tempConfigFile, err := ioutil.TempFile("", "pgbouncer-config-")
	assert.Nil(t, err, "failed to create temporary config file")

	return tempConfigFile
}
