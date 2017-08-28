package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"
)

func mockApp() *cli.App {
	app := cli.NewApp()
	app.Flags = []cli.Flag{cli.StringFlag{Name: "global"}}
	app.Commands = []cli.Command{
		{
			Name:   "command",
			Flags:  []cli.Flag{cli.StringFlag{Name: "local"}},
			Action: checkMissingFlags,
		},
	}

	return app
}

func TestCheckMissingFlags(t *testing.T) {
	t.Run("is successful when all flags present", func(t *testing.T) {
		err := mockApp().Run([]string{"", "--global", "g", "command", "--local", "l"})
		assert.Nil(t, err, "should not return error")
	})

	t.Run("returns error when missing global", func(t *testing.T) {
		err := mockApp().Run([]string{"", "command", "--local", "l"})
		if assert.Error(t, err, "did not return an error") {
			assert.Regexp(t, "Missing configuration flags.*global", err.Error())
		}
	})

	t.Run("returns error when missing local", func(t *testing.T) {
		err := mockApp().Run([]string{"", "--global", "g", "command"})
		if assert.Error(t, err, "did not return an error") {
			assert.Regexp(t, "Missing configuration flags.*local", err.Error())
		}
	})
}
