package pgbouncer

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/gocardless/pgsql-novips/errors"
	"github.com/stretchr/testify/assert"
)

func TestPsqlOptions_WithValidConfig(t *testing.T) {
	bouncer := pgBouncer{
		ConfigFile:         "/etc/pgbouncer/pgbouncer.ini",
		ConfigFileTemplate: "./fixtures/pgbouncer.ini.template",
	}

	options, err := bouncer.psqlOptions()

	assert.Nil(t, err, "expected no error")
	assert.Equal(t, options.Addr, "/var/run/postgresql/.s.PGSQL.6432")
}

func TestGenerateConfig_WithInvalidConfigTemplateErrors(t *testing.T) {
	bouncer := pgBouncer{
		ConfigFile:         "/etc/pgbouncer/pgbouncer.ini",
		ConfigFileTemplate: "/this/does/not/exist",
	}

	err := bouncer.GenerateConfig("curly.db.ams.gc.cx")
	assert.IsType(t, errors.ErrorWithFields{}, err, "expected error to be ErrorWithFields")

	ferr, _ := err.(errors.ErrorWithFields)

	assert.Error(t, err, "expected config generation to fail")
	assert.Equal(t, "failed to read PGBouncer config template file", err.Error())
	assert.Equal(t, "/this/does/not/exist", (*ferr.Fields)["path"])
}

func TestGenerateConfig_WritesConfigWithHost(t *testing.T) {
	tempConfigFile := makeTempFile(t, "pgbouncer-config-")
	defer os.Remove(tempConfigFile.Name())

	bouncer := pgBouncer{
		ConfigFile:         tempConfigFile.Name(),
		ConfigFileTemplate: "./fixtures/pgbouncer.ini.template",
	}

	err := bouncer.GenerateConfig("curly.db.ams.gc.cx")
	assert.Nil(t, err, "failed to generate config")

	configBuffer, _ := ioutil.ReadFile(tempConfigFile.Name())
	assert.Contains(t, string(configBuffer),
		"postgres = host=curly.db.ams.gc.cx", "expected host to be in generated config")
}

func makeTempFile(t *testing.T, prefix string) *os.File {
	tempFile, err := ioutil.TempFile("", prefix)
	assert.Nil(t, err, "failed to create temporary file")

	return tempFile
}
