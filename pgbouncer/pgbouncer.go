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
	"github.com/gocardless/pgsql-novips/errors"
)

// PGBouncer represents a set of configuration required to manage a PGBouncer instance,
// with a path to a config template that can be rendered.
type PGBouncer struct {
	ConfigFile         string
	ConfigFileTemplate string // template that can be rendered with Host value
}

// GenerateConfig writes new configuration to PGBouncer.ConfigFile
func (b *PGBouncer) GenerateConfig(host string) error {
	var configBuffer bytes.Buffer
	template, err := b.configTemplate()

	if err != nil {
		return err
	}

	err = template.Execute(&configBuffer, struct{ Host string }{host})

	if err != nil {
		return err
	}

	return ioutil.WriteFile(b.ConfigFile, configBuffer.Bytes(), 0644)
}

func (b *PGBouncer) configTemplate() (*template.Template, error) {
	configFile, err := ioutil.ReadFile(b.ConfigFileTemplate)

	if err != nil {
		return nil, errors.NewErrorWithFields(
			"failed to read PGBouncer config template file",
			&map[string]interface{}{
				"path":  b.ConfigFileTemplate,
				"error": err,
			},
		)
	}

	return template.Must(template.New("PGBouncerConfig").Parse(string(configFile))), err
}

type psqlExecutor interface {
	Exec(interface{}, ...interface{}) (orm.Result, error)
	WithTimeout(time.Duration) *pg.DB
}

func (b PGBouncer) psql() (psqlExecutor, error) {
	var psql psqlExecutor
	psqlOptions, err := b.psqlOptions()

	if err != nil {
		return psql, err
	}

	return pg.Connect(psqlOptions), err
}

func (b PGBouncer) psqlOptions() (*pg.Options, error) {
	var nullString string

	socketDir := b.configValue("unix_socket_dir")
	portStr := b.configValue("listen_port")
	port, _ := strconv.Atoi(strings.TrimSpace(portStr))

	if socketDir == nullString || portStr == nullString {
		return nil, errors.NewErrorWithFields(
			"failed to parse required config from PGBouncer config template",
			&map[string]interface{}{
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

func (b PGBouncer) configValue(key string) string {
	var value string

	configTemplateFile, err := os.Open(b.ConfigFileTemplate)

	if err != nil {
		return value
	}

	defer configTemplateFile.Close()

	r, _ := regexp.Compile(fmt.Sprintf("^%s\\s*\\=\\s*(\\S+)$", key))
	scanner := bufio.NewScanner(configTemplateFile)

	for scanner.Scan() {
		line := scanner.Text()
		if result := r.FindStringSubmatch(line); result != nil {
			return result[1]
		}
	}

	return value
}
