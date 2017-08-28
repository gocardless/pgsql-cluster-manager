package pgbouncer

import (
	"bufio"
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"
	"github.com/gocardless/pgsql-novips/util"
)

// PGBouncer provides an interface to interact with a local PGBouncer service
type PGBouncer interface {
	Config() (map[string]string, error)
	GenerateConfig(string) error
	Psql(time.Duration) (PsqlExecutor, error)
}

// PsqlExecutor implements the execution of SQL queries against a Postgres connection
type PsqlExecutor interface {
	Exec(interface{}, ...interface{}) (orm.Result, error)
}

// PGBouncer represents a set of configuration required to manage a PGBouncer
// instance, with a path to a config template that can be rendered.
type pgBouncer struct {
	ConfigFile         string
	ConfigFileTemplate string // template that can be rendered with Host value
}

// NewPGBouncer returns a PGBouncer configured around the given configFile and template
func NewPGBouncer(configFile, configFileTemplate string) PGBouncer {
	return &pgBouncer{
		ConfigFile:         configFile,
		ConfigFileTemplate: configFileTemplate,
	}
}

// Config generates a key value map of config parameters from the PGBouncer config
// template file
func (b pgBouncer) Config() (map[string]string, error) {
	config := make(map[string]string)
	configFile, err := os.Open(b.ConfigFileTemplate)

	if err != nil {
		return nil, util.NewErrorWithFields(
			"Failed to read PGBouncer config template file",
			map[string]interface{}{
				"path":  b.ConfigFileTemplate,
				"error": err,
			},
		)
	}

	defer configFile.Close()

	r, _ := regexp.Compile("^(\\S+)\\s*\\=\\s*(\\S+)$")
	scanner := bufio.NewScanner(configFile)

	for scanner.Scan() {
		line := scanner.Text()
		if result := r.FindStringSubmatch(line); result != nil {
			config[result[1]] = result[2]
		}
	}

	return config, nil
}

// GenerateConfig writes new configuration to PGBouncer.ConfigFile
func (b pgBouncer) GenerateConfig(host string) error {
	var configBuffer bytes.Buffer
	template, err := b.createTemplate()

	if err != nil {
		return err
	}

	err = template.Execute(&configBuffer, struct{ Host string }{host})

	if err != nil {
		return err
	}

	return ioutil.WriteFile(b.ConfigFile, configBuffer.Bytes(), 0644)
}

// Psql returns a connection to PGBouncers Postgres database that is configured with the
// specified timeout
func (b pgBouncer) Psql(timeout time.Duration) (PsqlExecutor, error) {
	var psql PsqlExecutor
	psqlOptions, err := b.psqlOptions()

	if err != nil {
		return psql, err
	}

	return pg.Connect(psqlOptions).WithTimeout(timeout), err
}

func (b pgBouncer) psqlOptions() (*pg.Options, error) {
	var nullString string
	var config map[string]string

	config, err := b.Config()

	if err != nil {
		return nil, err
	}

	socketDir := config["unix_socket_dir"]
	portStr := config["listen_port"]
	port, _ := strconv.Atoi(strings.TrimSpace(portStr))

	if socketDir == nullString || portStr == nullString {
		return nil, util.NewErrorWithFields(
			"Failed to parse required config from PGBouncer config template",
			map[string]interface{}{
				"socketDir":          socketDir,
				"portStr":            portStr,
				"port":               port,
				"configFileTemplate": b.ConfigFileTemplate,
			},
		)
	}

	return &pg.Options{
		Network:     "unix",
		User:        "pgbouncer",
		Database:    "pgbouncer",
		Addr:        fmt.Sprintf("%s/.s.PGSQL.%d", socketDir, port),
		ReadTimeout: time.Second,
	}, nil
}

func (b pgBouncer) createTemplate() (*template.Template, error) {
	configTemplate, err := ioutil.ReadFile(b.ConfigFileTemplate)

	if err != nil {
		return nil, util.NewErrorWithFields(
			"Failed to read PGBouncer config template file",
			map[string]interface{}{
				"path":  b.ConfigFileTemplate,
				"error": err,
			},
		)
	}

	return template.Must(template.New("PGBouncerConfig").Parse(string(configTemplate))), err
}
